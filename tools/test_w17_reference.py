"""W-17 differential reference runner: profile mapping, drift detection,
command scrubbing and container handling, all without Docker."""
import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

_PATH = Path(__file__).resolve().parents[1] / 'tests/vm/w17/reference.py'
_SPEC = importlib.util.spec_from_file_location('w17_reference', _PATH)
oracle = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(oracle)


def _fixture_tree(directory):
    root = Path(directory)
    (root / 'cases').mkdir()
    (root / 'cases/b_case.php').write_text('<?php echo 2;\n')
    (root / 'cases/a_case.php').write_text('<?php echo 1;\n')
    (root / 'reference.php').write_text('<?php\n')
    return root


def _payload(root, width=64, machine='aarch64'):
    cases = [{'case': p.stem, 'source_sha256': hashlib.sha256(p.read_bytes()).hexdigest(),
              'display': {}, 'log': {}, 'both': {}, 'return_json': None}
             for p in sorted((root / 'cases').glob('*.php'))]
    return {'php_version': '8.5.12', 'int_size': width // 8, 'os': 'Linux', 'machine': machine, 'cases': cases}


def _fake_docker(payload, calls, stderr=b'', image_present=True, run_error=None):
    def run(argv, **kwargs):
        calls.append(argv)
        if argv[1] == 'image':
            return subprocess.CompletedProcess(argv, 0 if image_present else 1, b'', b'')
        if argv[1] == 'pull':
            return subprocess.CompletedProcess(argv, 0, b'', b'Download complete\n')
        if argv[1] == 'run':
            if run_error:
                raise run_error
            return subprocess.CompletedProcess(argv, 0, json.dumps(payload).encode(), stderr)
        return subprocess.CompletedProcess(argv, 0, b'', b'')
    return run


class W17ReferenceTests(unittest.TestCase):
    def test_profiles_map_to_their_own_image_and_file(self):
        self.assertEqual(oracle.resolve_profile(64), (64, 'arm64'))
        self.assertEqual(oracle.resolve_profile(32), (32, '386'))
        self.assertEqual(oracle.resolve_profile(64, 'amd64'), (64, 'amd64'))
        self.assertEqual(oracle.reference_path(64).name, 'reference-64.json')
        self.assertEqual(oracle.reference_path(32).name, 'reference-32.json')
        self.assertEqual(oracle.reference_path(64, 'amd64').name, 'reference-64-amd64.json')
        self.assertEqual(oracle.reference_path(64, 'amd64', '/elsewhere'), Path('/elsewhere/reference-64-amd64.json'))
        self.assertEqual(len({image for _, image in oracle.IMAGES.values()}), 3)
        for invalid in [(32, 'amd64'), (32, 'arm64'), (64, '386')]:
            with self.subTest(profile=invalid), self.assertRaises(ValueError):
                oracle.resolve_profile(*invalid)

    def test_cli_rejects_unpinned_or_unknown_profiles(self):
        self.assertEqual(oracle.parse_args([]).profile, (64, 'arm64'))
        self.assertEqual(oracle.parse_args(['--width', '32']).profile, (32, '386'))
        args = oracle.parse_args(['--arch', 'amd64', '--write', '--output-dir', '/tmp/x'])
        self.assertEqual((args.profile, args.write, args.output_dir), ((64, 'amd64'), True, Path('/tmp/x')))
        for argv in (['--width', '32', '--arch', 'amd64'], ['--width', '16'], ['--arch', 'riscv64']):
            with self.subTest(argv=argv), contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as invalid:
                    oracle.parse_args(argv)
                self.assertEqual(invalid.exception.code, 2)

    def test_scrubbed_command_has_no_host_path_or_container_name(self):
        command = oracle.docker_command('linux/arm64', 'img', 'phego-w17-abc', Path('/Users/someone/checkout/tests/vm/w17'))
        scrubbed = oracle.scrub_command(command)
        text = json.dumps(scrubbed)
        self.assertNotIn('/Users/someone', text)
        self.assertNotIn('phego-w17-abc', text)
        self.assertIn('type=bind,src=<repo>/tests/vm/w17,dst=/w,readonly', scrubbed)
        self.assertIn('<unique-container-name>', scrubbed)
        self.assertEqual(command[0:2], ['docker', 'run'])
        self.assertNotEqual(command, scrubbed)
        for flag in ('--pull=never', '--network=none', '--read-only', '--user=65534:65534', '--cap-drop=ALL',
                     '--security-opt=no-new-privileges', '--pids-limit=64', '--memory=256m', '--cpus=1',
                     '--tmpfs=/tmp:rw,noexec,nosuid,size=8m'):
            self.assertIn(flag, command)
        self.assertEqual(command[-3:], ['php', '-n', '/w/reference.php'])

    def test_drift_names_changed_missing_and_extra_cases_but_ignores_machine(self):
        base = {'php_version': '8.5.12', 'machine': 'i686', 'image': 'x',
                'cases': [{'case': 'a', 'display': 1, 'log': 1}, {'case': 'b', 'display': 1}]}
        same = json.loads(json.dumps(base))
        same['machine'] = 'x86_64'
        self.assertEqual(oracle.drift(base, same), [])
        changed = json.loads(json.dumps(base))
        changed['cases'][0]['log'] = 2
        changed['cases'][1]['case'] = 'c'
        changed['image'] = 'y'
        self.assertEqual(oracle.drift(base, changed), [
            'top-level field image differs',
            'case a differs in log',
            'case b is missing from the regenerated reference',
            'case c is not in the committed reference',
        ])
        reordered = json.loads(json.dumps(base))
        reordered['cases'].reverse()
        self.assertEqual(oracle.drift(base, reordered), ['case order differs'])

    def test_compare_mode_fails_on_drift_and_preserves_committed_file(self):
        with tempfile.TemporaryDirectory() as directory:
            committed = Path(directory) / 'reference-64.json'
            original = oracle.encode({'cases': [{'case': 'a', 'display': 1}]}).encode()
            committed.write_bytes(original)
            with patch.object(oracle, 'FIXTURES', Path(directory)), \
                    contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()) as err:
                with patch.object(oracle, 'collect', return_value={'cases': [{'case': 'a', 'display': 2}]}):
                    self.assertEqual(oracle.main([]), 1)
                self.assertIn('case a differs in display', err.getvalue())
                self.assertEqual(committed.read_bytes(), original)
                with patch.object(oracle, 'collect', return_value={'cases': [{'case': 'a', 'display': 1}]}):
                    self.assertEqual(oracle.main([]), 0)
                for failure in (subprocess.TimeoutExpired('docker', 1), OSError('no docker'), ValueError('bad')):
                    with self.subTest(failure=failure), patch.object(oracle, 'collect', side_effect=failure):
                        self.assertEqual(oracle.main(['--write']), 1)
                        self.assertEqual(committed.read_bytes(), original)

    def test_write_mode_targets_profile_file_and_guards_concurrent_edits(self):
        with tempfile.TemporaryDirectory() as directory, tempfile.TemporaryDirectory() as out:
            fixtures = Path(directory)
            payload = {'cases': []}
            with patch.object(oracle, 'FIXTURES', fixtures), \
                    contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                for argv, name, profile in [(['--width', '64'], 'reference-64.json', (64, 'arm64')),
                                            (['--width', '32'], 'reference-32.json', (32, '386')),
                                            (['--arch', 'amd64'], 'reference-64-amd64.json', (64, 'amd64'))]:
                    with self.subTest(argv=argv):
                        with patch.object(oracle, 'collect', return_value=payload) as collect:
                            self.assertEqual(oracle.main(argv + ['--write']), 0)
                            collect.assert_called_once_with(*profile)
                        self.assertEqual(json.loads((fixtures / name).read_text()), payload)
                        with patch.object(oracle, 'collect', return_value=payload):
                            self.assertEqual(oracle.main(argv + ['--write', '--output-dir', out]), 0)
                        self.assertTrue((Path(out) / name).exists())

                        def concurrent(*_):
                            (fixtures / name).write_bytes(b'concurrent edit')
                            return payload
                        with patch.object(oracle, 'collect', side_effect=concurrent):
                            self.assertEqual(oracle.main(argv + ['--write']), 1)
                        self.assertEqual((fixtures / name).read_bytes(), b'concurrent edit')

    def test_collect_records_scrubbed_profile_metadata_and_pulls_only_missing_images(self):
        with tempfile.TemporaryDirectory() as directory:
            root = _fixture_tree(directory)
            for present, width, arch, machine in [(True, 64, None, 'aarch64'), (False, 32, None, 'i686'),
                                                  (True, 64, 'amd64', 'x86_64')]:
                calls = []
                run = _fake_docker(_payload(root, width, machine), calls, image_present=present)
                with self.subTest(width=width, arch=arch), patch.object(oracle.subprocess, 'run', side_effect=run):
                    data = oracle.collect(width, arch, root)
                    key = oracle.resolve_profile(width, arch)
                    self.assertEqual([c[1] for c in calls], ['image', 'run', 'rm'] if present else ['image', 'pull', 'run', 'rm'])
                    self.assertEqual((data['platform'], data['image']), oracle.IMAGES[key])
                    self.assertEqual(data['command'], oracle.scrub_command(calls[-2]))
                    self.assertEqual(list(data['inputs_sha256']), ['cases/a_case.php', 'cases/b_case.php', 'reference.php'])
                    self.assertEqual([c['case'] for c in data['cases']], ['a_case', 'b_case'])
                    self.assertNotIn(str(root), json.dumps(data))
                    if not present:
                        self.assertEqual(calls[1][2:4], ['--platform', 'linux/386'])

    def test_collect_rejects_runner_stderr_wrong_profile_and_mismatched_cases(self):
        with tempfile.TemporaryDirectory() as directory:
            root = _fixture_tree(directory)
            good = _payload(root)
            missing = dict(good, cases=good['cases'][:1])
            tampered = json.loads(json.dumps(good))
            tampered['cases'][0]['source_sha256'] = '0' * 64
            for label, payload, stderr in [('stderr', good, b'runner warning'),
                                           ('width', dict(good, int_size=4), b''),
                                           ('version', dict(good, php_version='8.5.11'), b''),
                                           ('machine', dict(good, machine='x86_64'), b''),
                                           ('missing case', missing, b''),
                                           ('case hash', tampered, b'')]:
                with self.subTest(label), patch.object(oracle.subprocess, 'run', side_effect=_fake_docker(payload, [], stderr)):
                    with self.assertRaises(ValueError):
                        oracle.collect(64, None, root)

    def test_timeout_removes_only_the_owned_container(self):
        with tempfile.TemporaryDirectory() as directory:
            root = _fixture_tree(directory)
            calls = []
            run = _fake_docker(None, calls, run_error=subprocess.TimeoutExpired('docker', 1))
            with patch.object(oracle.subprocess, 'run', side_effect=run), self.assertRaises(subprocess.TimeoutExpired):
                oracle.collect(64, None, root)
            command = next(c for c in calls if c[1] == 'run')
            name = command[command.index('--name') + 1]
            self.assertTrue(name.startswith('phego-w17-'))
            self.assertEqual(calls[-1], ['docker', 'rm', '-f', name])

    def test_committed_references_cover_current_inputs(self):
        inputs = oracle.input_hashes()
        names = sorted(Path(p).stem for p in inputs if p.startswith('cases/'))
        self.assertGreaterEqual(len(names), 60)
        for key, file in oracle.FILES.items():
            with self.subTest(profile=key):
                data = json.loads((oracle.FIXTURES / file).read_text())
                self.assertEqual(data['inputs_sha256'], inputs)
                self.assertEqual((data['platform'], data['image']), oracle.IMAGES[key])
                self.assertEqual(data['int_size'] * 8, key[0])
                self.assertIn(data['machine'], oracle.MACHINES[key])
                self.assertEqual([c['case'] for c in data['cases']], names)
                for case in data['cases']:
                    for mode in ('display', 'log', 'both'):
                        self.assertEqual(set(case[mode]), {'stdout_b64', 'stderr_b64', 'exit_code'})
                    self.assertEqual(case['return_json'] is None, not case['case'].startswith('return_'))


if __name__ == '__main__':
    unittest.main()
