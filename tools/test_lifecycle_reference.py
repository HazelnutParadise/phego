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
        name = calls[0][calls[0].index('--name') + 1]
        self.assertTrue(name.startswith('phego-w14-'))
        self.assertEqual(calls[1], ['docker', 'rm', '-f', name])
        self.assertIn('--network=none', calls[0])
        self.assertIn('--read-only', calls[0])
        self.assertIn('--user=65534:65534', calls[0])


if __name__ == '__main__':
    unittest.main()
