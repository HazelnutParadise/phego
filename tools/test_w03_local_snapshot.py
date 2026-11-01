"""Verify that local checks read the selected committed tree only."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import w03_local_gate


class SnapshotTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'repo'
        self.root.mkdir()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Local test')
        self.git('config', 'user.email', 'test@example.invalid')
        (self.root / 'tracked.txt').write_text('committed', encoding='utf-8')
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')
        self.commit = self.git('rev-parse', 'HEAD').strip()
        self.destination = Path(self.tmp.name) / 'snapshot'
        self.destination.mkdir()

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], text=True)

    def test_uses_commit_even_if_worktree_changes(self):
        (self.root / 'tracked.txt').write_text('changed', encoding='utf-8')
        tree = w03_local_gate._snapshot_commit(self.root, self.commit, self.destination)
        self.assertEqual((self.destination / 'tracked.txt').read_text(), 'committed')
        self.assertEqual(tree, self.git('rev-parse', 'HEAD^{tree}').strip())

    def test_rejects_missing_or_non_commit_revision(self):
        for revision in ('0' * 40, 'HEAD', self.git('hash-object', 'tracked.txt').strip()):
            with self.subTest(revision=revision), self.assertRaises(ValueError):
                w03_local_gate._snapshot_commit(self.root, revision, self.destination)

    def test_rejects_symlinks_in_committed_tree(self):
        os.symlink('tracked.txt', self.root / 'link')
        self.git('add', 'link')
        self.git('commit', '-qm', 'symlink')
        with self.assertRaises(ValueError):
            w03_local_gate._snapshot_commit(self.root, self.git('rev-parse', 'HEAD').strip(), self.destination)


    def test_snapshot_has_a_total_deadline(self):
        with patch('time.monotonic', side_effect=[0.0, 301.0]), self.assertRaises(ValueError):
            w03_local_gate._snapshot_commit(self.root, self.commit, self.destination)


if __name__ == '__main__':
    unittest.main()
