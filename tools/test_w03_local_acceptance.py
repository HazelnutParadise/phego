"""W-03 local-exit decision based on this run's observed tests."""
import copy
import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import w03_local_acceptance


class LocalExitTests(unittest.TestCase):
    def make_report(self):
        names = sorted(w03_local_acceptance.REQUIRED_CASES)
        events = []
        for name in names:
            events.extend([[name, 'start'], [name, 'pass'], [name, 'stop']])
        return {
            'commit': 'a' * 40,
            'expected_count': len(names),
            'observation': {'status': 'pass', 'cases': {name: 'pass' for name in names}, 'events': events},
            'package_acceptance': 'not_claimed', 'product_acceptance': 'not_claimed',
        }

    def test_complete_observation_can_satisfy_only_local_exit(self):
        self.assertTrue(w03_local_acceptance.observed_local_exit(self.make_report(), 0, 'a' * 40))

    def test_absent_or_failed_or_incomplete_observation_cannot_pass(self):
        base = self.make_report()
        mutations = []
        bad = copy.deepcopy(base); bad['observation']['status'] = 'timeout'; mutations.append(bad)
        bad = copy.deepcopy(base); bad['observation']['cases'][next(iter(bad['observation']['cases']))] = 'skip'; mutations.append(bad)
        bad = copy.deepcopy(base); bad['observation']['events'].pop(); mutations.append(bad)
        bad = copy.deepcopy(base); bad['observation']['cases'].pop(next(iter(bad['observation']['cases']))); mutations.append(bad)
        bad = copy.deepcopy(base); bad['expected_count'] += 1; mutations.append(bad)
        bad = copy.deepcopy(base); bad['commit'] = 'b' * 40; mutations.append(bad)
        bad = copy.deepcopy(base); bad['product_acceptance'] = 'pass'; mutations.append(bad)
        mutations.extend([None, {}, {'observation': None}])
        for bad in mutations:
            with self.subTest(bad=bad):
                self.assertFalse(w03_local_acceptance.observed_local_exit(bad, 0, 'a' * 40))
        self.assertFalse(w03_local_acceptance.observed_local_exit(base, 1, 'a' * 40))

    def test_completed_claim_must_have_matching_finished_observation(self):
        base = self.make_report()
        names = sorted(base['observation']['cases'])
        with self.subTest('event recorded for another test'):
            bad = copy.deepcopy(base); bad['observation']['events'][1][0] = names[1]
            self.assertFalse(w03_local_acceptance.observed_local_exit(bad, 0, 'a' * 40))
        with self.subTest('unexecuted mandatory test'):
            bad = copy.deepcopy(base); del bad['observation']['cases'][names[0]]
            bad['observation']['events'] = bad['observation']['events'][3:]
            bad['expected_count'] -= 1
            self.assertFalse(w03_local_acceptance.observed_local_exit(bad, 0, 'a' * 40))

    def test_unrelated_git_blob_cannot_supply_local_completion(self):
        script = Path(__file__).with_name('w03_local_acceptance.py')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                return subprocess.run(['git', '-C', str(root), *args], check=True, capture_output=True, text=True, timeout=20).stdout.strip()
            git('init', '-q')
            git('config', 'user.name', 'W03 test')
            git('config', 'user.email', 'w03-test@example.invalid')
            (root / 'evidence.txt').write_text('synthetic committed evidence\n', encoding='utf-8')
            git('add', 'evidence.txt')
            git('commit', '-qm', 'unrelated evidence')
            commit = git('rev-parse', 'HEAD')
            result = subprocess.run([sys.executable, '-I', str(script), '--root', str(root), '--commit', commit], capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 2)
            self.assertNotIn('"status": "pass"', result.stdout)

    def test_uncommitted_gate_cannot_forge_execution(self):
        script = Path(__file__).with_name('w03_local_acceptance.py')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); tools = root / 'tools'; tools.mkdir()
            shutil.copyfile(script, tools / script.name)
            (tools / 'w03_local_gate.py').write_text('def main():\n    return 1\n', encoding='utf-8')
            def git(*args):
                return subprocess.run(['git', '-C', str(root), *args], check=True, capture_output=True, text=True, timeout=30).stdout.strip()
            git('init', '-q'); git('config', 'user.name', 'W03 test'); git('config', 'user.email', 'w03-test@example.invalid')
            git('add', 'tools'); git('commit', '-qm', 'committed gate'); commit = git('rev-parse', 'HEAD')
            report = self.make_report(); report['commit'] = commit
            fake = "import json\nfrom pathlib import Path\ndef _git(root, *args): return Path(__file__).with_name('w03_local_acceptance.py').read_bytes()\ndef main():\n    print(" + repr(json.dumps(report)) + ")\n    return 0\n"
            (tools / 'w03_local_gate.py').write_text(fake, encoding='utf-8')
            result = subprocess.run([sys.executable, '-I', str(tools / script.name), '--root', str(root), '--commit', commit], capture_output=True, text=True, timeout=30)
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertNotIn('"status": "pass"', result.stdout)

    def test_nonisolated_cli_is_rejected(self):
        script = Path(__file__).with_name('w03_local_acceptance.py')
        result = subprocess.run([sys.executable, str(script), '--commit', 'a' * 40], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 2)
        self.assertIn('-I', result.stderr)

    def test_isolated_cli_ignores_uncommitted_shadow_modules(self):
        script = Path(__file__).with_name('w03_local_acceptance.py')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            shutil.copyfile(script, root / script.name)
            payload = "print('FORGED PASS'); raise SystemExit(0)\n"
            for name in ('json.py', '__future__.py'):
                (root / name).write_text(payload, encoding='utf-8')
            result = subprocess.run([sys.executable, '-I', str(root / script.name), '--commit', 'a' * 40], capture_output=True, text=True, timeout=20)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn('FORGED PASS', result.stdout)

    def test_cli_does_not_accept_external_report(self):
        script = Path(__file__).with_name('w03_local_acceptance.py')
        result = subprocess.run([sys.executable, '-I', str(script), '--commit', 'a' * 40, '--results', 'fake.json'], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 2)
        self.assertIn('unrecognized arguments', result.stderr)


if __name__ == '__main__':
    unittest.main()
