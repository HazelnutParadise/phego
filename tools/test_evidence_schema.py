"""Keep the published evidence-record schema aligned with the validator."""
import json
import unittest
from pathlib import Path
import traceability

ROOT = Path(__file__).resolve().parents[1]

class EvidenceSchemaContract(unittest.TestCase):
    def test_outcomes_and_profile_fields_match_validator(self):
        schema = json.loads((ROOT / "docs/data/evidence-records.schema.json").read_text())
        self.assertEqual(schema["$schema"], "https://json-schema.org/draft/2020-12/schema")
        record = schema["$defs"]["record"]
        self.assertEqual(set(record["properties"]["status"]["enum"]), set(traceability._OUTCOMES))
        profile = schema["$defs"]["profile"]
        self.assertEqual(set(profile["required"]), set(traceability._PROFILE_FIELDS))
        self.assertFalse(profile["additionalProperties"])

if __name__ == "__main__":
    unittest.main()
