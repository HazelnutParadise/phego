"""End-to-end checks for the W-03 local execution observer."""
import os
import signal
import subprocess
import tempfile
import unittest
from pathlib import Path

import w03_local_gate


class ObservedExecutionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root / 'tools').mkdir()
        (self.root / 'tools' / 'test_fixture.py').write_text('''\
import pathlib, subprocess, sys, time, unittest
class Cases(unittest.TestCase):
    def test_ok(self): self.assertEqual(2 + 2, 4)
    def test_bad(self): self.fail("actual failure")
    @unittest.skip("not executed")
    def test_skipped(self): pass
    def test_slow(self): time.sleep(10)
    def test_subtests(self):
        with self.subTest(value=1): self.assertEqual(1, 2)
        with self.subTest(value=2): self.assertEqual(2, 2)
    def test_subskip(self):
        with self.subTest(value=1): self.skipTest("subtest unavailable")
    def test_spawn(self):
        child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(20)'])
        pathlib.Path(__file__).with_name('child.pid').write_text(str(child.pid))
''', encoding='utf-8')

    def run_cases(self, names, timeout=15):
        return w03_local_gate._observe_cases(self.root, names, timeout)

    def test_real_success_is_observed_and_does_not_claim_product_acceptance(self):
        result = self.run_cases(['test_fixture.Cases.test_ok'])
        self.assertEqual(result['status'], 'pass')
        self.assertEqual(result['cases'], {'test_fixture.Cases.test_ok': 'pass'})
        self.assertEqual(result['events'], [['test_fixture.Cases.test_ok', 'start'], ['test_fixture.Cases.test_ok', 'pass'], ['test_fixture.Cases.test_ok', 'stop']])
        self.assertEqual(result['product_acceptance'], 'not_claimed')

    def test_snapshot_package_imports_work_for_discovery_and_execution(self):
        (self.root / 'tools' / 'helper.py').write_text('VALUE = 42\n', encoding='utf-8')
        (self.root / 'tools' / 'test_package_import.py').write_text(
            'import unittest\nfrom tools import helper\nclass Cases(unittest.TestCase):\n'
            '    def test_import(self): self.assertEqual(helper.VALUE, 42)\n', encoding='utf-8')
        name = 'test_package_import.Cases.test_import'
        self.assertIn(name, w03_local_gate._discover_cases(self.root, 15))
        self.assertEqual(self.run_cases([name])['status'], 'pass')

    def test_failure_and_skip_are_not_pass(self):
        for name in ('test_bad', 'test_skipped'):
            with self.subTest(name=name):
                result = self.run_cases(['test_fixture.Cases.' + name])
                self.assertNotEqual(result['status'], 'pass')
                if name == 'test_bad':
                    self.assertIn('actual failure', result['diagnostics'])

    def test_missing_or_empty_or_duplicate_cases_are_rejected(self):
        for names in ([], ['test_fixture.Cases.missing'],
                      ['test_fixture.Cases.test_ok'] * 2):
            with self.subTest(names=names):
                result = self.run_cases(names)
                self.assertNotEqual(result['status'], 'pass')

    def test_class_and_module_cleanup_errors_cannot_pass(self):
        for kind in ('class', 'module'):
            with self.subTest(kind=kind):
                fixture = self.root / 'tools' / 'test_cleanup.py'
                fixture.write_text(
                    "import unittest\nclass Cases(unittest.TestCase):\n"
                    "    def test_ok(self): pass\n" +
                    ("    @classmethod\n    def tearDownClass(cls): raise RuntimeError('cleanup failed')\n"
                     if kind == 'class' else
                     "def tearDownModule(): raise RuntimeError('cleanup failed')\n"), encoding='utf-8')
                result = self.run_cases(['test_cleanup.Cases.test_ok'])
                self.assertNotEqual(result['status'], 'pass')
                self.assertIn('cleanup failed', result['diagnostics'])

    def test_timeout_retains_finished_case_and_output(self):
        fixture = self.root / 'tools' / 'test_partial.py'
        fixture.write_text("import time, unittest\nclass Cases(unittest.TestCase):\n"
                           "    def test_done(self): print('done output', flush=True)\n"
                           "    def test_wait(self): print('wait output', flush=True); time.sleep(120)\n",
                           encoding='utf-8')
        names = ['test_partial.Cases.test_done', 'test_partial.Cases.test_wait']
        result = self.run_cases(names, timeout=30)
        self.assertEqual(result['status'], 'timeout')
        self.assertEqual(result['cases'][names[0]], 'pass')
        self.assertIn('done output', result['diagnostics'])
        self.assertIn('wait output', result['diagnostics'])

    def test_timeout_never_becomes_pass(self):
        result = self.run_cases(['test_fixture.Cases.test_slow'], timeout=1)
        self.assertEqual(result['status'], 'timeout')
        self.assertEqual(result['product_acceptance'], 'not_claimed')

    def test_completed_suite_does_not_leave_background_children(self):
        result = self.run_cases(['test_fixture.Cases.test_spawn'])
        self.assertEqual(result['status'], 'pass')
        pid = int((self.root / 'tools' / 'child.pid').read_text())
        proc = subprocess.run(['ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True)
        alive = proc.returncode == 0 and proc.stdout.strip() and not proc.stdout.strip().startswith('Z')
        if alive:
            os.kill(pid, signal.SIGKILL)
        self.assertFalse(alive, 'completed suite left a descendant running')

    def test_subtest_failure_keeps_each_parent_case_result(self):
        names = ['test_fixture.Cases.test_ok', 'test_fixture.Cases.test_subtests']
        result = self.run_cases(names)
        self.assertEqual(result['status'], 'fail')
        self.assertEqual(result['cases'], {names[0]: 'pass', names[1]: 'fail'})
        self.assertIn('1 != 2', result['diagnostics'])


    def test_subtest_skip_is_not_counted_as_pass(self):
        name = 'test_fixture.Cases.test_subskip'
        result = self.run_cases([name])
        self.assertEqual(result['status'], 'fail')
        self.assertEqual(result['cases'][name], 'skip')


if __name__ == '__main__':
    unittest.main()
