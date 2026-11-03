"""The local check list must be explicit, bounded, and cannot omit core checks."""
import json
import tempfile
import unittest
from pathlib import Path

import w03_local_gate


class ManifestTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / 'checks.json'
        self.core = [
            'test_work_status.WorkStatusTest.test_requirement_with_missing_test_is_rejected',
            'test_work_status.WorkStatusTest.test_dependency_cycle_in_plan_is_rejected',
            'test_w03_local_gate.ObservedExecutionTests.test_missing_or_empty_or_duplicate_cases_are_rejected',
        ]

    def read(self, value):
        self.path.write_text(json.dumps(value), encoding='utf-8')
        return w03_local_gate._read_manifest(self.path)

    def test_explicit_core_cases_are_accepted(self):
        data = {'schema_version': 1, 'scope': 'W-03 local tool checks; not product acceptance', 'cases': self.core}
        self.assertEqual(self.read(data), self.core)

    def test_empty_duplicate_unknown_fields_or_missing_core_fail(self):
        base = {'schema_version': 1, 'scope': 'W-03 local tool checks; not product acceptance', 'cases': self.core}
        for value in ({**base, 'cases': []}, {**base, 'cases': self.core + self.core[:1]},
                      {**base, 'cases': self.core[:-1]}, {**base, 'unknown': True},
                      {**base, 'schema_version': True}):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.read(value)

    def test_duplicate_json_keys_fail(self):
        self.path.write_text('{"schema_version":1,"schema_version":1}', encoding='utf-8')
        with self.assertRaises(ValueError):
            w03_local_gate._read_manifest(self.path)


if __name__ == '__main__':
    unittest.main()
