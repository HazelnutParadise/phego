"""Check that imported claims cannot be passed as local execution evidence."""
import os
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
from pathlib import Path

import w03_local_gate


class LocalGateCLITests(unittest.TestCase):
    def test_help_explains_scope_and_trusted_source(self):
        p = subprocess.run([sys.executable, w03_local_gate.__file__, '--help'], capture_output=True, text=True)
        self.assertEqual(p.returncode, 0)
        self.assertIn('not a security sandbox', p.stdout)
        self.assertIn('--commit', p.stdout)

    def test_imported_results_are_not_accepted(self):
        p = subprocess.run([sys.executable, w03_local_gate.__file__, '--commit', '0' * 40,
                            '--results', 'claims.json'], capture_output=True, text=True)
        self.assertNotEqual(p.returncode, 0)
        self.assertIn('unrecognized arguments', p.stderr)

    def test_snapshot_fingerprint_changes_when_source_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'file').write_text('first')
            before = w03_local_gate._snapshot_fingerprint(root)
            (root / 'file').write_text('second')
            self.assertNotEqual(before, w03_local_gate._snapshot_fingerprint(root))

    def test_snapshot_rejects_oversized_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'file').write_bytes(b'123456789')
            with patch.object(w03_local_gate, '_MAX_FILE', 8), self.assertRaises(ValueError):
                w03_local_gate._snapshot_fingerprint(root)

    def test_snapshot_fingerprint_detects_added_directories(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            before = w03_local_gate._snapshot_fingerprint(root)
            (root / 'added').mkdir()
            self.assertNotEqual(before, w03_local_gate._snapshot_fingerprint(root))

    def test_discovery_timeout_stops_inherited_pipe_processes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'tools').mkdir()
            (root / 'tools' / 'test_hang.py').write_text(
                'import pathlib, subprocess, sys, time\n'
                'child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(40)"])\n'
                'pathlib.Path(__file__).with_name("child.pid").write_text(str(child.pid))\n'
                'time.sleep(40)\n')
            started = time.monotonic()
            with self.assertRaises(ValueError):
                w03_local_gate._discover_cases(root, 10)
            self.assertLess(time.monotonic() - started, 20)
            pid = int((root / 'tools' / 'child.pid').read_text())
            proc = subprocess.run(['ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True)
            alive = proc.returncode == 0 and proc.stdout.strip() and not proc.stdout.strip().startswith('Z')
            if alive:
                os.kill(pid, signal.SIGKILL)
            self.assertFalse(alive, 'discovery left a descendant running')

    def test_successful_discovery_does_not_leave_background_children(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'tools').mkdir()
            (root / 'tools' / 'test_spawn.py').write_text(
                'import pathlib, subprocess, sys, unittest\n'
                'child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(20)"])\n'
                'pathlib.Path(__file__).with_name("child.pid").write_text(str(child.pid))\n'
                'class Cases(unittest.TestCase):\n'
                '    def test_ok(self): pass\n')
            self.assertEqual(w03_local_gate._discover_cases(root, 15), ['test_spawn.Cases.test_ok'])
            pid = int((root / 'tools' / 'child.pid').read_text())
            proc = subprocess.run(['ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True)
            alive = proc.returncode == 0 and proc.stdout.strip() and not proc.stdout.strip().startswith('Z')
            if alive:
                os.kill(pid, signal.SIGKILL)
            self.assertFalse(alive, 'successful discovery left a descendant running')


if __name__ == '__main__':
    unittest.main()
