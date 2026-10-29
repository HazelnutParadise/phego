"""Check that recorded source revisions are actual commits in this repository."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import check_evidence_commit


class EvidenceCommitTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        subprocess.run(['git', 'init', '-q', str(self.root)], check=True)
        subprocess.run(['git', '-C', str(self.root), 'config', 'user.email', 'test@example.invalid'], check=True)
        subprocess.run(['git', '-C', str(self.root), 'config', 'user.name', 'Test'], check=True)
        (self.root / 'docs/data').mkdir(parents=True)
        (self.root / 'docs/data/phego-acceptance-tests.json').write_text(json.dumps({'items': [{'id': 'T-001'}, {'id': 'T-002'}]}))
        (self.root / 'source').write_text('source')
        subprocess.run(['git', '-C', str(self.root), 'add', '.'], check=True)
        subprocess.run(['git', '-C', str(self.root), 'commit', '-qm', 'test'], check=True)
        self.commit = subprocess.check_output(['git', '-C', str(self.root), 'rev-parse', 'HEAD'], text=True).strip()
        (self.root / 'output').write_text('observed')
        self.records = self.root / 'records.json'
        self.record = {'test_id': 'T-001', 'status': 'pass', 'commit': self.commit,
                       'command': 'test command', 'profile': {'release': '8.5.12', 'platform': 'linux/amd64', 'build': 'pure-go'},
                       'exit_code': 0, 'evidence': {'path': 'output', 'sha256': hashlib.sha256(b'observed').hexdigest()}}

    def report(self, records):
        self.records.write_text(json.dumps({'records': records}))
        return check_evidence_commit.verify(self.root, self.records)

    def test_real_commit_and_complete_denominator(self):
        result = self.report([self.record])
        self.assertEqual(result['commit'], self.commit)
        self.assertEqual(result['commit_check'], 'exists')
        self.assertEqual(result['counts']['not_run'], 1)
        self.assertEqual(result['product_acceptance'], 'not_claimed')
        self.assertEqual(result['execution_verification'], 'not_verified')

    def test_missing_commit_is_rejected(self):
        self.record['commit'] = 'a' * 40
        with self.assertRaises(ValueError): self.report([self.record])

    def test_blob_is_not_a_commit(self):
        self.record['commit'] = subprocess.check_output(['git', '-C', str(self.root), 'hash-object', '-w', 'source'], text=True).strip()
        with self.assertRaises(ValueError): self.report([self.record])

    def test_empty_records_are_not_claimed_verified(self):
        result = self.report([])
        self.assertIsNone(result['commit'])
        self.assertEqual(result['commit_check'], 'not_applicable')
        self.assertEqual(result['counts']['not_run'], 2)

    def test_parent_repository_cannot_stand_in_for_root(self):
        child = self.root / 'nested'; child.mkdir()
        with self.assertRaises(ValueError): check_evidence_commit.verify(child, None)

    def test_git_environment_cannot_redirect_lookup(self):
        old = os.environ.get('GIT_DIR')
        self.addCleanup(lambda: os.environ.pop('GIT_DIR', None) if old is None else os.environ.__setitem__('GIT_DIR', old))
        os.environ['GIT_DIR'] = '/nonexistent'
        self.assertEqual(self.report([self.record])['commit_check'], 'exists')

    def test_cli_help(self):
        p = subprocess.run([sys.executable, str(Path(check_evidence_commit.__file__)), '--help'], capture_output=True, text=True)
        self.assertEqual(p.returncode, 0)
        self.assertIn('--results', p.stdout)


if __name__ == '__main__':
    unittest.main()
