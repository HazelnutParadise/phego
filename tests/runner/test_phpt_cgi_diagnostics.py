"""CGI profile failures report a safe, actionable category."""
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "tools"))
import phpt_execute as executor


class CGIProfileDiagnosticTest(unittest.TestCase):
    def test_profile_failure_categories_do_not_disclose_output(self):
        cases = {"timeout": "timed out", "output_limit": "output limit",
                 "profile_mismatch": "does not match", "runner_gap": "could not be verified"}
        sections = {"TEST": b"safe", "FILE": b"<?php ?>", "EXPECT": b"", "CGI": b""}
        for status, phrase in cases.items():
            with self.subTest(status=status):
                profile = SimpleNamespace(status="completed", output=b"8.5.12\n4", exit_code=0)
                cgi = SimpleNamespace(status=status, output=b"sensitive-output", exit_code=1,
                                      reason="sensitive-reason")
                case = SimpleNamespace(status="runner_gap", cleanup_confirmed=True, profile=profile,
                                       cgi_profile=cgi, skipif=None, program=None, clean_phase=None)
                with patch.object(executor, "run_case", return_value=case):
                    result = executor.run(sections, image="test@sha256:" + "0" * 64,
                                          executable="/usr/local/bin/php", platform="linux/386",
                                          release="8.5.12", timeout=10, output_limit=4096,
                                          cgi_executable="/usr/local/bin/php-cgi")
                self.assertEqual(result["status"], "runner_gap")
                self.assertIn(phrase, result["reason"])
                self.assertNotIn("sensitive", result["reason"])

if __name__ == "__main__":
    unittest.main()
