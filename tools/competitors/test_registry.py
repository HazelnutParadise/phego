"""Security and integration tests for the frozen competitor-plan registry."""
import hashlib
import json
import os
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest import mock

try:
    from . import registry
except ImportError:
    import registry


class RegistryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.root = self.base / "input"
        self.root.mkdir()
        self.bundle = self.base / "bundle.phego"
        self.payload = b"controlled fixture\n"
        (self.root / "source.txt").write_bytes(self.payload)
        self.digest = hashlib.sha256(self.payload).hexdigest()
        self.plan = {
            "version": 1,
            "competitors": ["engine"],
            "workloads": ["work"],
            "platforms": ["linux"],
            "modes": ["safe"],
            "cells": [{
                "competitor": "engine", "workload": "work",
                "platform": "linux", "mode": "safe",
                "status": "comparable", "evidence": "checked",
            }],
            "files": [{"path": "source.txt", "sha256": self.digest}],
        }

    def assert_rejected(self, plan=None, *, root=None, output=None, bundle=None):
        with self.assertRaises(ValueError) as caught:
            if bundle is not None:
                registry.verify(bundle)
            elif output is not None:
                registry.freeze(root or self.root, plan or self.plan, output)
            elif isinstance(plan, Path):
                registry.main(["validate", "--root", str(root or self.root), "--plan", str(plan)])
            else:
                registry.validate(root or self.root, plan or self.plan)
        message = str(caught.exception)
        self.assertTrue(message.strip(), "errors must be explicit")
        self.assertNotIn("controlled fixture", message)
        self.assertNotIn("source.txt", message)
        return message

    def patch_file_writes(self, after_write):
        """Inject behavior through the OS file-object boundary, not registry helpers."""
        real_fdopen = os.fdopen

        def controlled_fdopen(fd, *args, **kwargs):
            stream = real_fdopen(fd, *args, **kwargs)

            class ControlledStream:
                def write(self, data):
                    result = stream.write(data)
                    after_write(fd, data)
                    return result

                def __enter__(self):
                    return self

                def __exit__(self, *exc):
                    return stream.__exit__(*exc)

                def __getattr__(self, name):
                    return getattr(stream, name)

            return ControlledStream()

        return mock.patch.object(registry.os, "fdopen", side_effect=controlled_fdopen)

    def test_validate_and_freeze_and_verify_after_source_is_moved(self):
        snapshot = registry.validate(self.root, self.plan)
        self.assertIsNotNone(snapshot)
        registry.freeze(self.root, self.plan, self.bundle)
        shutil.rmtree(self.root)
        blockers = registry.verify(self.bundle)
        self.assertEqual(blockers, 0)

    def test_pending_cell_remains_a_blocker(self):
        plan = json.loads(json.dumps(self.plan))
        plan["cells"][0]["status"] = "pending"
        plan["cells"][0]["evidence"] = ""
        registry.freeze(self.root, plan, self.bundle)
        self.assertGreater(registry.verify(self.bundle), 0)

    def test_cartesian_product_missing_cell_is_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["competitors"].append("engine2")
        self.assert_rejected(plan)

    def test_duplicate_cell_is_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["cells"].append(dict(plan["cells"][0]))
        self.assert_rejected(plan)

    def test_unsupported_without_evidence_is_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["cells"][0].update(status="unsupported", evidence="")
        self.assert_rejected(plan)

    def test_duplicate_ids_are_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["workloads"].append("work")
        self.assert_rejected(plan)

    def test_unknown_cell_id_is_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["cells"][0]["competitor"] = "unknown"
        self.assert_rejected(plan)

    def test_false_digest_is_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["files"][0]["sha256"] = "0" * 64
        self.assert_rejected(plan)

    def test_source_change_between_validation_and_freeze_is_rejected(self):
        registry.validate(self.root, self.plan)
        (self.root / "source.txt").write_bytes(b"changed private fixture")
        self.assert_rejected(output=self.bundle)

    def test_archived_bundle_tampering_is_rejected(self):
        registry.freeze(self.root, self.plan, self.bundle)
        self.assertTrue(self.bundle.is_file(), "freeze must create one archive file")
        self.bundle.write_bytes(self.bundle.read_bytes() + b"tampered")
        self.assert_rejected(bundle=self.bundle)

    def test_repeated_invalid_archives_do_not_leak_file_descriptors(self):
        fd_directory = next(
            (path for path in ("/proc/self/fd", "/dev/fd") if os.path.isdir(path)),
            None,
        )
        if fd_directory is None:
            self.skipTest("no supported process file descriptor directory")

        empty = self.base / "empty.phego"
        empty.write_bytes(b"")
        too_short = self.base / "short.phego"
        too_short.write_bytes(b"x")

        for archive in (empty, too_short):
            for _ in range(25):
                before = len(os.listdir(fd_directory))
                with self.assertRaises(ValueError):
                    registry.verify(archive)
                after = len(os.listdir(fd_directory))
                self.assertEqual(after, before, "verify leaked a file descriptor")

    def test_path_traversal_is_rejected(self):
        plan = json.loads(json.dumps(self.plan))
        plan["files"][0]["path"] = "../outside.txt"
        self.assert_rejected(plan)

    def test_symlink_is_rejected(self):
        outside = self.base / "outside.txt"
        outside.write_bytes(self.payload)
        (self.root / "linked.txt").symlink_to(outside)
        plan = json.loads(json.dumps(self.plan))
        plan["files"][0]["path"] = "linked.txt"
        self.assert_rejected(plan)

    def test_existing_destination_is_rejected_without_overwrite(self):
        marker = self.bundle
        marker.write_text("owned", encoding="utf-8")
        self.assert_rejected(output=self.bundle)
        self.assertEqual(marker.read_text(encoding="utf-8"), "owned")

    def test_replacing_archive_name_after_creation_is_rejected_and_preserved(self):
        replaced = False

        def replace_after_write(_fd, _data):
            nonlocal replaced
            if not replaced and self.bundle.exists():
                replaced = True
                self.bundle.rename(self.bundle.with_suffix(".displaced"))
                self.bundle.write_bytes(b"replacement")

        with self.patch_file_writes(replace_after_write):
            self.assert_rejected(output=self.bundle)
        self.assertTrue(replaced, "the test must replace the output name after a write")
        self.assertEqual(self.bundle.read_bytes(), b"replacement")

    def test_replacing_archive_name_during_exclusive_creation_is_rejected(self):
        real_open = registry.os.open
        replaced = False

        def replace_during_create(path, flags, *args, **kwargs):
            nonlocal replaced
            fd = real_open(path, flags, *args, **kwargs)
            if (not replaced and path == self.bundle.name
                    and flags & os.O_CREAT and flags & os.O_EXCL):
                replaced = True
                self.bundle.rename(self.bundle.with_suffix(".displaced"))
                self.bundle.write_bytes(b"replacement")
            return fd

        with mock.patch.object(registry.os, "open", side_effect=replace_during_create):
            self.assert_rejected(output=self.bundle)
        self.assertTrue(replaced, "the test must replace the name during exclusive creation")
        self.assertEqual(self.bundle.read_bytes(), b"replacement")

    def test_failed_archive_write_leaves_partial_file_path(self):
        failed = False

        def fail_after_first_byte(fd, data):
            nonlocal failed
            if not failed:
                failed = True
                os.ftruncate(fd, 1)
                raise OSError("injected write failure")

        with self.patch_file_writes(fail_after_first_byte):
            self.assert_rejected(output=self.bundle)
        self.assertTrue(failed, "the test must inject an OS file write failure")
        self.assertTrue(self.bundle.is_file(), "a failed write must leave its partial archive")
        self.assertEqual(self.bundle.stat().st_size, 1)

    def test_cli_freeze_and_verify_use_single_archive_path(self):
        plan_path = self.base / "plan.json"
        plan_path.write_text(json.dumps(self.plan), encoding="utf-8")
        with mock.patch("sys.stdout"):
            registry.main(["freeze", "--root", str(self.root), "--plan", str(plan_path),
                           "--output", str(self.bundle)])
            registry.main(["verify", "--bundle", str(self.bundle)])
        self.assertTrue(self.bundle.is_file())

    def test_duplicate_json_key_is_rejected(self):
        plan_path = self.base / "plan.json"
        plan_path.write_text('{"version":1,"version":1}', encoding="utf-8")
        self.assert_rejected(plan=plan_path)


if __name__ == "__main__":
    unittest.main()
