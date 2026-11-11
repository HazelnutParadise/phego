"""REQ-020/REQ-040; T-039/T-040/T-079/T-080: oracle integrity and failure reporting."""
import contextlib
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import lifecycle_reference as oracle


class LifecycleReferenceTests(unittest.TestCase):
    def test_reference_contains_every_fixture_with_exact_hash_and_separate_streams(self):
        data = json.loads(oracle.EXPECTED.read_text())
        files = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in oracle.FIXTURES.glob('*.php')}
        self.assertEqual(data['inputs_sha256'], files)
        self.assertEqual(data['image'], oracle.IMAGE)
        names = set(files) - {'common.php', 'reference.php'}
        self.assertEqual({c['case'] + '.php' for c in data['cases']}, names)
        self.assertEqual(len(data['cases']), len(names))
        for case in data['cases']:
            self.assertIsInstance(case['exit_code'], int)
            for field in ('stdout', 'stderr', 'combined'):
                self.assertIsInstance(case[field], str)

    def test_failure_or_drift_is_failure_and_preserves_expected_result(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'reference.json'
            original = b'{"cases": []}\n'
            path.write_bytes(original)
            with patch.object(oracle, 'EXPECTED', path), contextlib.redirect_stderr(io.StringIO()):
                for failure in (subprocess.TimeoutExpired('docker', 60), OSError('docker unavailable'),
                                ValueError('invalid oracle')):
                    with self.subTest(failure=failure), patch.object(oracle, 'collect', side_effect=failure):
                        self.assertEqual(oracle.main([]), 1)
                        self.assertEqual(path.read_bytes(), original)
                with patch.object(oracle, 'collect', return_value={'cases': [{'exit_code': 7}]}):
                    self.assertEqual(oracle.main([]), 1)
                    self.assertEqual(path.read_bytes(), original)

    def test_timeout_removes_only_owned_container(self):
        calls = []

        def run(argv, **kwargs):
            calls.append(argv)
            if argv[1] == 'run':
                raise subprocess.TimeoutExpired(argv, kwargs['timeout'])
            return subprocess.CompletedProcess(argv, 0, b'', b'')

        with patch.object(oracle.subprocess, 'run', side_effect=run):
            with self.assertRaises(subprocess.TimeoutExpired):
                oracle.collect()
        command = next(c for c in calls if c[1] == 'run')
        name = command[command.index('--name') + 1]
        self.assertTrue(name.startswith('phego-w14-'))
        self.assertEqual(calls[-1], ['docker', 'rm', '-f', name])
        self.assertIn('--network=none', command)
        self.assertIn('--read-only', command)
        self.assertIn('--user=65534:65534', command)

    def test_cold_image_pull_noise_is_separate_from_reference_streams(self):
        for width in (32, 64):
            calls = []
            payload = {'php_version': '8.5.12', 'int_size': width // 8,
                       'os': 'Linux', 'cases': [{'stderr': 'PHP warning\n'}]}

            def run(argv, **kwargs):
                calls.append(argv)
                if argv[1] == 'image':
                    return subprocess.CompletedProcess(argv, 1, b'', b'missing')
                if argv[1] == 'run':
                    return subprocess.CompletedProcess(argv, 0, json.dumps(payload).encode(), b'')
                return subprocess.CompletedProcess(argv, 0, b'', b'Download complete\n')

            with self.subTest(width=width), patch.object(oracle.subprocess, 'run', side_effect=run):
                data = oracle.collect(width)
                self.assertEqual(data['cases'], payload['cases'])
                self.assertEqual([c[1] for c in calls], ['image', 'pull', 'run', 'rm'])
                self.assertEqual(calls[1][2:4], ['--platform', 'linux/386' if width == 32 else 'linux/arm64'])
                self.assertIn('--pull=never', calls[2])

    def test_runner_stderr_and_wrong_profile_are_rejected(self):
        for stderr, size in [(b'runner warning', 8), (b'', 4)]:
            def run(argv, **kwargs):
                payload = {'php_version': '8.5.12', 'int_size': size, 'os': 'Linux', 'cases': []}
                return subprocess.CompletedProcess(argv, 0, json.dumps(payload).encode(), stderr if argv[1] == 'run' else b'')
            with self.subTest(stderr=stderr, size=size), patch.object(oracle.subprocess, 'run', side_effect=run):
                with self.assertRaises(ValueError):
                    oracle.collect()

    def test_pull_failure_does_not_start_container(self):
        calls = []
        def run(argv, **kwargs):
            calls.append(argv)
            if argv[1] == 'image':
                return subprocess.CompletedProcess(argv, 1, b'', b'missing')
            raise subprocess.CalledProcessError(1, argv, stderr=b'pull failed')
        with patch.object(oracle.subprocess, 'run', side_effect=run):
            with self.assertRaises(subprocess.CalledProcessError):
                oracle.collect()
        self.assertEqual([c[1] for c in calls], ['image', 'pull'])

    def test_both_profiles_cover_identical_fixtures_and_observed_behavior(self):
        wide = json.loads(oracle.EXPECTED.read_text())
        narrow = json.loads((oracle.FIXTURES / 'reference-32.json').read_text())
        self.assertEqual(narrow['int_size'], 4)
        self.assertEqual(narrow['image'], oracle.IMAGES[(32, '386')][1])
        self.assertEqual(narrow['inputs_sha256'], wide['inputs_sha256'])
        self.assertEqual(narrow['cases'], wide['cases'])

    def test_cli_width_selects_own_record_and_guards_concurrent_changes(self):
        with tempfile.TemporaryDirectory() as directory:
            fixtures = Path(directory)
            wide = fixtures / 'reference.json'
            narrow = fixtures / 'reference-32.json'
            amd64 = fixtures / 'reference-amd64.json'
            payload = {'cases': []}
            original = json.dumps(payload).encode()
            for path in (wide, narrow, amd64):
                path.write_bytes(original)
            with patch.object(oracle, 'EXPECTED', wide), patch.object(oracle, 'FIXTURES', fixtures), \
                    contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                # REQ-040; T-079/T-080. amd64 shares arm64's width but is its
                # own profile with its own record.
                for argv, width, arch, path in [(['--width', '64'], 64, None, wide), (['--width', '32'], 32, None, narrow),
                                                (['--width', '64', '--arch', 'amd64'], 64, 'amd64', amd64)]:
                    with patch.object(oracle, 'collect', return_value=payload) as collect:
                        self.assertEqual(oracle.main(argv), 0)
                        collect.assert_called_once_with(width, arch)
                    def changed(*_):
                        path.write_bytes(b'concurrent edit')
                        return payload
                    with patch.object(oracle, 'collect', side_effect=changed):
                        self.assertEqual(oracle.main(argv + ['--write']), 1)
                        self.assertEqual(path.read_bytes(), b'concurrent edit')
                    path.write_bytes(original)
                with self.assertRaises(SystemExit) as invalid:
                    oracle.main(['--width', '32', '--arch', 'amd64'])
                self.assertEqual(invalid.exception.code, 2)
                with self.assertRaises(SystemExit) as invalid:
                    oracle.main(['--width', '16'])
                self.assertEqual(invalid.exception.code, 2)


if __name__ == '__main__':
    unittest.main()
