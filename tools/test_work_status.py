"""Regression tests for work-package status claims and their Git evidence."""
from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
from pathlib import Path

import work_status


PROJECT_ROOT = Path(__file__).resolve().parents[1]
TOOL = PROJECT_ROOT / "tools" / "work_status.py"


class WorkStatusTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.base = Path(self.temp.name)
        self.root = self.base / "project"
        shutil.copytree(PROJECT_ROOT, self.root, ignore=shutil.ignore_patterns(".git", "__pycache__"))
        self.run_git("init", "-q")
        self.run_git("config", "user.name", "Work status test")
        self.run_git("config", "user.email", "work-status-test@example.invalid")
        (self.root / "evidence.txt").write_text("synthetic committed evidence\n", encoding="utf-8")
        self.run_git("add", "evidence.txt")
        self.run_git("commit", "-q", "-m", "synthetic evidence")
        self.commit = self.run_git("rev-parse", "HEAD").stdout.strip()
        self.digest = hashlib.sha256(b"synthetic committed evidence\n").hexdigest()
        self.ledger_path = self.base / "ledger.json"

    def tearDown(self) -> None:
        self.temp.cleanup()

    def run_git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args], cwd=self.root, text=True, capture_output=True, check=True
        )

    def evidence(self, *, commit: str | None = None, path: str = "evidence.txt", sha256: str | None = None) -> dict:
        return {"commit": commit or self.commit, "path": path, "sha256": sha256 or self.digest}

    def event(self, package: str = "W-01", status: str = "in_progress", **overrides: object) -> dict:
        event = {
            "id": package,
            "status": status,
            "reason": "合成狀態轉移測試",
            "evidence": self.evidence(),
        }
        event.update(overrides)
        return event

    def write_ledger(self, events: object) -> Path:
        self.ledger_path.write_text(json.dumps({"events": events}), encoding="utf-8")
        return self.ledger_path

    def report(self, events: list[dict] | None = None) -> dict:
        ledger = self.write_ledger(events) if events is not None else None
        return work_status.status_report(self.root, ledger)

    @staticmethod
    def status(report: dict, package: str) -> str:
        return report["statuses"][package]

    def test_empty_ledger_reports_plan_as_not_started_without_claiming_current_state(self) -> None:
        report = self.report([])
        self.assertGreaterEqual(len(report["statuses"]), 60)
        self.assertTrue(all(status == "not_started" for status in report["statuses"].values()))
        self.assertEqual(report["events"], [])
        self.assertEqual(report["status_basis"], "planning_defaults_and_unverified_claims")
        plan = self.root / "docs/data/phego-work-packages.json"
        self.assertEqual(report["planning_input_sha256"]["docs/data/phego-work-packages.json"], hashlib.sha256(plan.read_bytes()).hexdigest())
        self.assertEqual(report["verification_status"], "not_verified")
        self.assertEqual(report["product_acceptance"], "not_claimed")
        self.assertEqual(report["prerequisites"]["W-03"]["dependencies"], ["W-01", "W-02"])
        self.assertEqual(report["prerequisites"]["W-03"]["integration_dependencies"], [])
        self.assertEqual(report["prerequisites"]["W-35"]["integration_dependencies"], ["W-25", "W-26"])

    def test_all_supported_initial_transitions_require_reason_and_real_evidence(self) -> None:
        for status in ("in_progress", "blocked", "claimed_complete"):
            with self.subTest(status=status):
                report = self.report([self.event(status=status)])
                self.assertEqual(self.status(report, "W-01"), status)

    def test_dependency_completion_and_claimed_completion_require_all_dependencies(self) -> None:
        with self.assertRaises((ValueError, TypeError)):
            self.report([self.event("W-02", "claimed_complete")])

        report = self.report([
            self.event("W-01", "claimed_complete"),
            self.event("W-02", "claimed_complete"),
            self.event("W-03", "in_progress"),
        ])
        self.assertEqual(self.status(report, "W-03"), "in_progress")

        with self.assertRaises((ValueError, TypeError)):
            self.report([
                self.event("W-01", "claimed_complete"),
                self.event("W-03", "claimed_complete"),
            ])

    def test_integration_prerequisites_cannot_be_omitted(self) -> None:
        plan = json.loads((self.root / "docs/data/phego-work-packages.json").read_text())
        index = json.loads((self.root / "docs/implementation/github-index.json").read_text())
        packages = {item["id"]: item for item in plan["items"]}
        indexed = {item["id"]: item for item in index["items"]}
        events = []
        seen = set()
        def add_complete(package):
            if package in seen:
                return
            for dependency in packages[package]["dependencies"] + indexed[package]["integration_dependencies"]:
                add_complete(dependency)
            seen.add(package)
            events.append(self.event(package, "claimed_complete"))
        for dependency in packages["W-35"]["dependencies"]:
            add_complete(dependency)
        self.assertNotIn("W-25", seen)
        self.assertNotIn("W-26", seen)
        with self.assertRaises((ValueError, TypeError)):
            self.report(events + [self.event("W-35", "claimed_complete")])
        add_complete("W-25")
        add_complete("W-26")
        self.assertEqual(self.status(self.report(events + [self.event("W-35", "claimed_complete")]), "W-35"), "claimed_complete")

    def test_transitions_reject_duplicates_and_allow_only_documented_reopens(self) -> None:
        valid = [
            self.event(status="in_progress"),
            self.event(status="blocked"),
            self.event(status="in_progress"),
            self.event(status="claimed_complete"),
            self.event(status="in_progress"),
        ]
        self.assertEqual(self.status(self.report(valid), "W-01"), "in_progress")
        for sequence in (
            [self.event(), self.event()],
            [self.event(status="blocked"), self.event(status="claimed_complete")],
            [self.event(status="claimed_complete"), self.event(status="blocked")],
            [self.event(status="in_progress"), self.event(status="not_started")],
        ):
            with self.subTest(sequence=[e["status"] for e in sequence]):
                with self.assertRaises((ValueError, TypeError)):
                    self.report(sequence)

    def test_every_event_requires_nonempty_reason_and_evidence(self) -> None:
        bad_events = [
            self.event(reason=""),
            self.event(reason="  "),
            self.event(evidence=None),
            self.event(evidence={"commit": self.commit, "path": "evidence.txt"}),
            self.event(evidence={"path": "evidence.txt", "sha256": self.digest}),
        ]
        for event in bad_events:
            with self.subTest(event=event):
                with self.assertRaises((ValueError, TypeError)):
                    self.report([event])

    def test_rejects_unknown_ids_and_malformed_ledger_shapes(self) -> None:
        for events in ([self.event("W-999")], {}, ["not an event"], [self.event(status="unknown")]):
            with self.subTest(events=events):
                with self.assertRaises((ValueError, TypeError)):
                    self.report(events)  # type: ignore[arg-type]
        self.write_ledger(None)
        with self.assertRaises((ValueError, TypeError)):
            work_status.status_report(self.root, self.ledger_path)
        self.ledger_path.write_text("{broken", encoding="utf-8")
        with self.assertRaises((ValueError, TypeError)):
            work_status.status_report(self.root, self.ledger_path)

    def test_evidence_must_be_a_real_commit_with_a_tracked_regular_blob_and_matching_digest(self) -> None:
        invalid_evidence = [
            self.evidence(commit="0" * 40),
            self.evidence(commit="HEAD"),
            self.evidence(path="missing.txt"),
            self.evidence(sha256="0" * 64),
        ]
        for evidence in invalid_evidence:
            with self.subTest(evidence=evidence):
                with self.assertRaises((ValueError, TypeError)):
                    self.report([self.event(evidence=evidence)])

    def test_annotated_tag_object_is_not_a_commit_id(self) -> None:
        self.run_git("tag", "-a", "synthetic-tag", "-m", "tag")
        tag_object = self.run_git("rev-parse", "synthetic-tag").stdout.strip()
        with self.assertRaises(ValueError):
            self.report([self.event(evidence=self.evidence(commit=tag_object))])

    def test_evidence_path_rejects_absolute_traversal_and_symlink(self) -> None:
        outside = self.base / "outside.txt"
        outside.write_text("outside\n", encoding="utf-8")
        link = self.root / "evidence-link"
        link.symlink_to("evidence.txt")
        self.run_git("add", "evidence-link")
        self.run_git("commit", "-q", "-m", "synthetic symlink evidence")
        self.assertEqual(link.readlink(), Path("evidence.txt"))
        link.unlink()  # Exercise committed tree mode independently of worktree checks.
        symlink_commit = self.run_git("rev-parse", "HEAD").stdout.strip()
        link_digest = hashlib.sha256(b"evidence.txt").hexdigest()
        with self.assertRaises((ValueError, TypeError)):
            self.report([self.event(evidence=self.evidence(commit=symlink_commit, path="evidence-link", sha256=link_digest))])
        bad_paths = [str(self.root / "evidence.txt"), "../outside.txt"]
        for path in bad_paths:
            with self.subTest(path=path):
                with self.assertRaises((ValueError, TypeError)):
                    self.report([self.event(evidence=self.evidence(path=path))])

    def test_git_environment_cannot_redirect_evidence_to_another_repository(self) -> None:
        with patch.dict(os.environ, {"GIT_DIR": str(self.base / "missing-git-dir"), "GIT_WORK_TREE": str(self.root)}):
            report = self.report([self.event()])
        self.assertEqual(self.status(report, "W-01"), "in_progress")

    def test_worktree_changes_do_not_change_commit_evidence_validation(self) -> None:
        (self.root / "evidence.txt").write_text("modified after commit\n", encoding="utf-8")
        report = self.report([self.event()])
        self.assertEqual(self.status(report, "W-01"), "in_progress")

    def test_reopening_dependency_requires_downstream_claims_to_be_reopened_too(self) -> None:
        claims = [
            self.event("W-01", "claimed_complete"),
            self.event("W-02", "claimed_complete"),
            self.event("W-03", "claimed_complete"),
            self.event("W-01", "in_progress"),
        ]
        with self.assertRaises((ValueError, TypeError)):
            self.report(claims)
        claims.insert(-1, self.event("W-03", "in_progress"))
        claims.insert(-1, self.event("W-02", "in_progress"))
        report = self.report(claims)
        self.assertEqual(self.status(report, "W-01"), "in_progress")
        self.assertEqual(self.status(report, "W-02"), "in_progress")
        self.assertEqual(self.status(report, "W-03"), "in_progress")

    def test_reopening_invalidates_transitive_claims_until_each_is_reopened(self) -> None:
        events = [self.event("W-01", "claimed_complete"), self.event("W-02", "claimed_complete"),
                  self.event("W-03", "claimed_complete"), self.event("W-01", "in_progress"),
                  self.event("W-01", "claimed_complete"), self.event("W-02", "in_progress"),
                  self.event("W-02", "claimed_complete")]
        with self.assertRaises(ValueError):
            self.report(events)
        events.extend([self.event("W-03", "in_progress"), self.event("W-03", "claimed_complete")])
        self.assertEqual(self.status(self.report(events), "W-03"), "claimed_complete")

    def test_requirement_with_missing_test_is_rejected(self) -> None:
        path = self.root / "docs/data/phego-requirements.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        data["items"][0]["tests"].pop()
        path.write_text(json.dumps(data), encoding="utf-8")
        with self.assertRaises(ValueError):
            self.report([])

    def test_recompleted_dependency_does_not_revalidate_stale_downstream_claim(self) -> None:
        claims = [self.event("W-01", "claimed_complete"), self.event("W-02", "claimed_complete"),
                  self.event("W-01", "in_progress"), self.event("W-01", "claimed_complete")]
        with self.assertRaises(ValueError):
            self.report(claims)
        claims.extend([self.event("W-02", "in_progress"), self.event("W-02", "claimed_complete")])
        self.assertEqual(self.status(self.report(claims), "W-02"), "claimed_complete")

    def test_dependency_cycle_in_plan_is_rejected(self) -> None:
        plan_path = self.root / "docs/data/phego-work-packages.json"
        plan = json.loads(plan_path.read_text(encoding="utf-8"))
        packages = {item["id"]: item for item in plan["items"]}
        packages["W-01"]["dependencies"] = ["W-02"]
        plan_path.write_text(json.dumps(plan), encoding="utf-8")
        with self.assertRaises((ValueError, TypeError)):
            work_status.status_report(self.root, None)

    def test_malformed_catalog_reports_input_error_without_traceback(self) -> None:
        path = self.root / "docs/data/competitor-catalog.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        data["items"] = [None]
        path.write_text(json.dumps(data), encoding="utf-8")
        result = subprocess.run([sys.executable, str(TOOL), "--root", str(self.root)],
                                text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("Traceback", result.stderr)

    def test_fifo_ledger_is_rejected_without_waiting_for_a_writer(self) -> None:
        fifo = self.base / "ledger.fifo"
        os.mkfifo(fifo)
        result = subprocess.run([sys.executable, str(TOOL), "--root", str(self.root), "--ledger", str(fifo)],
                                text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("Traceback", result.stderr)

    def test_fifo_markdown_in_project_cannot_hang_planning_validation(self) -> None:
        fifo = self.root / "ledger.md"
        os.mkfifo(fifo)
        result = subprocess.run([sys.executable, str(TOOL), "--root", str(self.root), "--ledger", str(fifo)],
                                text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("Traceback", result.stderr)

    def test_plan_swapped_to_fifo_after_validation_is_rejected_without_hanging(self) -> None:
        code = """
import os, pathlib, sys
sys.path.insert(0, sys.argv[1])
import work_status
original = work_status._validate_planning
def replace_plan(root):
    original(root)
    path = root / "docs/data/phego-work-packages.json"
    path.read_bytes()
    path.unlink()
    os.mkfifo(path)
work_status._validate_planning = replace_plan
try:
    work_status.status_report(pathlib.Path(sys.argv[2]), None)
except ValueError:
    sys.exit(2)
sys.exit(0)
"""
        result = subprocess.run([sys.executable, "-c", code, str(TOOL.parent), str(self.root)],
                                text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 2, result.stderr)

    def test_plan_content_changed_after_validation_is_rejected(self) -> None:
        original = work_status._validate_planning
        def replace_requirements(root):
            original(root)
            path = root / "docs/data/phego-requirements.json"
            data = json.loads(path.read_text(encoding="utf-8"))
            data["items"][0]["tests"] = []
            path.write_text(json.dumps(data), encoding="utf-8")
        with patch.object(work_status, "_validate_planning", side_effect=replace_requirements):
            with self.assertRaises(ValueError):
                self.report([])

    def test_planning_snapshot_does_not_read_unrelated_secret_file(self) -> None:
        (self.root / ".env").write_text("synthetic-secret-never-read", encoding="utf-8")
        original_open = os.open
        def guarded_open(path, flags, *args, **kwargs):
            if os.fspath(path) == ".env":
                raise AssertionError("unrelated .env contents must not be read")
            return original_open(path, flags, *args, **kwargs)
        with patch.object(work_status.os, "open", side_effect=guarded_open):
            report = self.report([])
        self.assertEqual(report["verification_status"], "not_verified")

    def test_planning_snapshot_preserves_linked_nonmarkdown_chapter_anchor(self) -> None:
        (self.root / "anchor.txt").write_text('<a id="chapter-01"></a>', encoding="utf-8")
        readme = self.root / "README.md"
        readme.write_text(readme.read_text(encoding="utf-8") + "\n[Anchor](anchor.txt#chapter-01)\n", encoding="utf-8")
        self.assertEqual(self.report([])["verification_status"], "not_verified")

    def test_status_ledger_rejects_excessive_bytes_and_event_count(self) -> None:
        self.ledger_path.write_bytes(b" " * (1024 * 1024 + 1))
        with self.assertRaisesRegex(ValueError, "1 MiB"):
            work_status.status_report(self.root, self.ledger_path)
        self.write_ledger([None] * 10001)
        with self.assertRaisesRegex(ValueError, "10000 events"):
            work_status.status_report(self.root, self.ledger_path)

    def test_committed_evidence_larger_than_limit_is_rejected(self) -> None:
        contents = b"x" * (8 * 1024 * 1024 + 1)
        (self.root / "large-evidence.txt").write_bytes(contents)
        self.run_git("add", "large-evidence.txt")
        self.run_git("commit", "-q", "-m", "large synthetic evidence")
        commit = self.run_git("rev-parse", "HEAD").stdout.strip()
        evidence = self.evidence(commit=commit, path="large-evidence.txt", sha256=hashlib.sha256(contents).hexdigest())
        with self.assertRaisesRegex(ValueError, "8 MiB"):
            self.report([self.event(evidence=evidence)])

    def test_cli_valid_json_exits_zero_and_invalid_ledger_exits_two(self) -> None:
        valid = subprocess.run(
            [sys.executable, str(TOOL), "--root", str(self.root)],
            text=True, capture_output=True,
        )
        self.assertEqual(valid.returncode, 0, valid.stderr)
        self.assertEqual(json.loads(valid.stdout)["product_acceptance"], "not_claimed")

        self.write_ledger([self.event(status="not_started")])
        invalid = subprocess.run(
            [sys.executable, str(TOOL), "--root", str(self.root), "--ledger", str(self.ledger_path)],
            text=True, capture_output=True,
        )
        self.assertEqual(invalid.returncode, 2)
        self.assertEqual(invalid.stdout, "")


if __name__ == "__main__":
    unittest.main()
