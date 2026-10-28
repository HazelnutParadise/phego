"""Behavioral tests for the source license manifest CLI."""

import json
import hashlib
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]
CLI = REPO / "tools" / "source_license_manifest.py"
INPUTS = {
    "compat.lock.json": REPO / "compat.lock.json",
    "docs/implementation/evidence/W-02-php-license-files.json": REPO
    / "docs/implementation/evidence/W-02-php-license-files.json",
    "docs/implementation/evidence/W-02-go-source-license-files.json": REPO
    / "docs/implementation/evidence/W-02-go-source-license-files.json",
    "docs/implementation/evidence/W-02-binary-license-files.json": REPO
    / "docs/implementation/evidence/W-02-binary-license-files.json",
    "docs/implementation/evidence/W-02-probe-image-packages.json": REPO
    / "docs/implementation/evidence/W-02-probe-image-packages.json",
    "docs/implementation/evidence/W-02-probe-images-20261029/debian-index.json": REPO
    / "docs/implementation/evidence/W-02-probe-images-20261029/debian-index.json",
    "docs/implementation/evidence/W-02-probe-images-20261029/buildpack-index.json": REPO
    / "docs/implementation/evidence/W-02-probe-images-20261029/buildpack-index.json",
    "docs/implementation/evidence/W-02-build-image-packages.json": REPO
    / "docs/implementation/evidence/W-02-build-image-packages.json",
}


class SourceLicenseManifestTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name) / "repo"
        self.make_repo(self.repo)

    @staticmethod
    def make_repo(repo):
        for relative, source in INPUTS.items():
            target = repo / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, target)

        # Keep the real evidence shapes while making each fixture small.
        php_path = repo / "docs/implementation/evidence/W-02-php-license-files.json"
        php = json.loads(php_path.read_text(encoding="utf-8"))
        php["files"] = php["files"][:3]
        php_path.write_text(json.dumps(php), encoding="utf-8")

        go_path = repo / "docs/implementation/evidence/W-02-go-source-license-files.json"
        go = json.loads(go_path.read_text(encoding="utf-8"))
        go["files"] = go["files"][:3]
        go_path.write_text(json.dumps(go), encoding="utf-8")

        # Binary inventories are archive captures. Keep their complete real
        # records so fixture identities and candidate files remain evidence-backed.

        packages_path = repo / "docs/implementation/evidence/W-02-build-image-packages.json"
        packages = json.loads(packages_path.read_text(encoding="utf-8"))
        for profile in packages["profiles"].values():
            profile["packages"] = profile["packages"][:2]
            profile["package_count"] = 2
        packages_path.write_text(json.dumps(packages), encoding="utf-8")

    def write_json(self, relative, value):
        path = self.repo / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value), encoding="utf-8")

    def read_json(self, relative):
        return json.loads((self.repo / relative).read_text(encoding="utf-8"))

    @staticmethod
    def probe_stdout_digest(packages):
        rows = []
        for package in packages:
            copyright = package["copyright"]
            columns = (
                package["package"],
                package["version"],
                copyright["path"],
                copyright["status"],
                copyright.get("resolved_path", "-"),
                str(copyright.get("size", "-")),
                copyright.get("sha256", "-"),
            )
            rows.append("\t".join(columns) + "\n")
        return hashlib.sha256("".join(rows).encode("utf-8")).hexdigest()

    def run_cli(self, *args):
        # A missing implementation must fail the test, never count as a valid
        # rejection of malformed input.
        self.assertTrue(CLI.is_file(), f"CLI is not implemented: {CLI}")
        return subprocess.run(
            [sys.executable, str(CLI), *args],
            cwd=REPO,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

    def generate(self, repo=None):
        return self.run_cli("generate", "--repo", str(repo or self.repo))

    def assert_rejected_without_json(self, repo=None):
        result = self.generate(repo)
        self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        self.assertEqual(result.stdout, b"", "failed generation must not emit partial JSON")
        self.assertNotIn(b"Traceback", result.stderr)
        return result

    def test_generate_check_and_reproducible_output_across_repo_paths(self):
        first = self.generate()
        self.assertEqual(first.returncode, 0, first.stderr.decode("utf-8", "replace"))
        manifest = json.loads(first.stdout)
        self.assertEqual(manifest.get("status"), "partial")
        self.assertIs(type(manifest.get("schema_version")), int)
        for key in ("artifacts", "inputs", "gaps"):
            self.assertIn(key, manifest)

        second_path = Path(self.temp.name) / "another-root" / "repo"
        self.make_repo(second_path)
        second = self.generate(second_path)
        self.assertEqual(second.returncode, 0, second.stderr.decode("utf-8", "replace"))
        self.assertEqual(first.stdout, second.stdout)

        manifest_path = self.repo / "manifest.json"
        manifest_path.write_bytes(first.stdout)
        checked = self.run_cli("check", "--repo", str(self.repo), str(manifest_path))
        self.assertEqual(checked.returncode, 0, checked.stderr.decode("utf-8", "replace"))

    def test_check_rejects_changed_input(self):
        generated = self.generate()
        self.assertEqual(generated.returncode, 0, generated.stderr.decode("utf-8", "replace"))
        manifest_path = self.repo / "manifest.json"
        manifest_path.write_bytes(generated.stdout)

        lock = self.read_json("compat.lock.json")
        lock["purpose"] += " changed"
        self.write_json("compat.lock.json", lock)
        checked = self.run_cli("check", "--repo", str(self.repo), str(manifest_path))
        self.assertNotEqual(checked.returncode, 0)

    def test_manifest_covers_locked_sources_and_reference_images(self):
        result = self.generate()
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        manifest = json.loads(result.stdout)
        rendered = json.dumps(manifest, sort_keys=True)
        lock = self.read_json("compat.lock.json")

        expected_digests = {
            lock["php"]["archive_sha256"],
            lock["go"]["archive_sha256"],
            lock["go"]["source_build"]["archive_sha256"],
            *(archive["archive_sha256"] for archive in lock["go"]["additional_archives"]),
            lock["llvm"]["archive_sha256"],
        }
        for digest in expected_digests:
            self.assertIn(digest, rendered)
        expected_platforms = {
            lock["go"]["platform"],
            *(archive["platform"] for archive in lock["go"]["additional_archives"]),
        }
        for platform in expected_platforms:
            self.assertIn(platform, rendered)

        def locked_images(value):
            if isinstance(value, dict):
                for key, item in value.items():
                    if key in {"image", "probe_image"} and isinstance(item, str):
                        yield item
                    yield from locked_images(item)
            elif isinstance(value, list):
                for item in value:
                    yield from locked_images(item)

        for image in locked_images(lock):
            self.assertIn(image, rendered)

        for relative in (
            "docs/implementation/evidence/W-02-php-license-files.json",
            "docs/implementation/evidence/W-02-go-source-license-files.json",
        ):
            evidence = self.read_json(relative)
            for record in evidence["files"]:
                self.assertIn(record["path"], rendered)
                self.assertIn(record["sha256"], rendered)

        package_evidence = self.read_json(
            "docs/implementation/evidence/W-02-build-image-packages.json"
        )
        probe_package_evidence = self.read_json(
            "docs/implementation/evidence/W-02-probe-image-packages.json"
        )
        recorded_probe_images = {
            (item["image"], item["platform"])
            for item in probe_package_evidence["inventories"]
            if item["role"] in {"go_probe", "llvm_probe"}
        }
        for relative in INPUTS:
            self.assertIn(relative, rendered)
        for platform, profile in package_evidence["profiles"].items():
            self.assertIn(platform, rendered)
            for record in profile["packages"]:
                self.assertIn(record["package"], rendered)
                self.assertIn(record["copyright"]["path"], rendered)
                self.assertIn(record["copyright"]["sha256"], rendered)

        gaps = manifest["gaps"]
        binary_evidence = self.read_json("docs/implementation/evidence/W-02-binary-license-files.json")
        recorded = {
            (item["component"], item["platform"], item["archive_url"], item["archive_sha256"])
            for item in binary_evidence["inventories"]
        }
        for kind in ("go_binaries", "llvm"):
            archives = manifest["artifacts"][kind]
            if kind == "llvm":
                archives = archives["archives"]
            for archive in archives:
                platform = archive["platform"]
                component = "go" if kind == "go_binaries" else "llvm"
                identity = (component, platform, archive["archive_url"], archive["archive_sha256"])
                if identity in recorded:
                    self.assertEqual(archive["license_inventory_status"], "partial_filename_inventory")
                    self.assertEqual(archive["license_applicability"], "unknown")
                else:
                    self.assertEqual(archive["license_inventory_status"], "not_recorded")
                    self.assertTrue(any(
                        gap.get("scope") == f"{'Go binary' if kind == 'go_binaries' else 'LLVM binary'} license inventory {platform}"
                        and gap.get("status") == "unknown"
                        for gap in gaps
                    ))
                if archive.get("probe_image"):
                    probe_identity = (archive["probe_image"], platform)
                    probe_gaps = [
                        gap for gap in gaps
                        if gap.get("image") == archive["probe_image"]
                        and gap.get("platform") == platform
                        and gap.get("status") == "unknown"
                    ]
                    if probe_identity in recorded_probe_images:
                        self.assertFalse(probe_gaps)
                    else:
                        self.assertTrue(probe_gaps)

        arm64_profile = next(
            item for item in manifest["artifacts"]["reference_build_images"]
            if item["platform"] == lock["reference_build"]["platform"]
        )
        self.assertEqual(arm64_profile["image_index"], package_evidence["image_index"])
        mapping_gap = next(
            gap for gap in gaps
            if gap.get("scope") == "reference image index mapping linux/arm64"
        )
        self.assertEqual(mapping_gap["status"], "unknown")
        self.assertIn("not been independently verified", mapping_gap["reason"])

    def test_binary_license_inventories_match_all_locked_archives(self):
        result = self.generate()
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        manifest = json.loads(result.stdout)
        lock = self.read_json("compat.lock.json")
        evidence = self.read_json("docs/implementation/evidence/W-02-binary-license-files.json")
        self.assertEqual(evidence["schema_version"], 1)
        self.assertEqual(evidence["status"], "partial")
        self.assertEqual(len(evidence["inventories"]), 7)

        expected = {}
        for component in ("go", "llvm"):
            locked = lock[component]
            rows = [locked, *locked.get("additional_archives", [])]
            for row in rows:
                expected[(component, locked["version"], row["platform"], row["archive_url"], row["archive_sha256"])] = row
        self.assertEqual(len(expected), 7)
        inventories = {
            (item["component"], item["version"], item["platform"], item["archive_url"], item["archive_sha256"]): item
            for item in evidence["inventories"]
        }
        self.assertEqual(set(inventories), set(expected))

        output_archives = [
            *manifest["artifacts"]["go_binaries"],
            *manifest["artifacts"]["llvm"]["archives"],
        ]
        for key, inventory in inventories.items():
            component, version, platform, archive_url, digest = key
            archive = next(
                item for item in output_archives
                if item.get("archive_url") == archive_url and item.get("platform") == platform
            )
            self.assertEqual(archive["version"], version)
            self.assertEqual(archive["archive_sha256"], digest)
            self.assertEqual(archive["license_inventory_status"], "partial_filename_inventory")
            self.assertEqual(archive["license_applicability"], "unknown")
            self.assertEqual(archive["candidate_files"], inventory["files"])

        old_inventory_gaps = {
            gap.get("scope") for gap in manifest["gaps"]
            if gap.get("scope", "").startswith(("Go binary license inventory ", "LLVM binary license inventory "))
        }
        self.assertFalse(old_inventory_gaps)

    def test_rejects_binary_license_inventory_omissions_duplicates_unknown_and_mismatches(self):
        relative = "docs/implementation/evidence/W-02-binary-license-files.json"
        original = self.read_json(relative)
        cases = []
        cases.append(("missing inventory", {**original, "inventories": original["inventories"][:-1]}))
        cases.append(("duplicate inventory", {**original, "inventories": [*original["inventories"], original["inventories"][0]]}))
        cases.append(("unknown inventory", {**original, "inventories": [*original["inventories"], {**original["inventories"][0], "platform": "linux/ppc64"}]}))
        for field, value in (("archive_sha256", "0" * 64), ("platform", "linux/ppc64")):
            changed = json.loads(json.dumps(original))
            changed["inventories"][0][field] = value
            cases.append((f"mismatched {field}", changed))

        for name, evidence in cases:
            with self.subTest(case=name):
                self.write_json(relative, evidence)
                self.assert_rejected_without_json()

    def test_rejects_invalid_binary_license_inventory_candidate_records_and_totals(self):
        relative = "docs/implementation/evidence/W-02-binary-license-files.json"
        original = self.read_json(relative)

        # The captured evidence itself must remain a valid input before each
        # corruption case is applied.
        valid = self.generate()
        self.assertEqual(valid.returncode, 0, valid.stderr.decode("utf-8", "replace"))

        cases = []
        for name, path in (
            ("parent traversal", "../go/LICENSE"),
            ("wrong archive root", "not-go/LICENSE"),
            ("absolute candidate path", "/go/LICENSE"),
        ):
            changed = json.loads(json.dumps(original))
            changed["inventories"][0]["files"][0]["path"] = path
            cases.append((name, changed))
        for name, encoding in (("null encoding", None), ("empty encoding", ""), ("invalid encoding", "utf16")):
            changed = json.loads(json.dumps(original))
            changed["inventories"][0]["files"][0]["encoding"] = encoding
            cases.append((name, changed))

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["format"] = "zip"
        cases.append(("format does not match archive", changed))

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["entries"] = len(changed["inventories"][0]["files"]) - 1
        cases.append(("candidate count exceeds entries", changed))

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["expanded_bytes"] = sum(
            item["size"] for item in changed["inventories"][0]["files"]
        ) - 1
        cases.append(("candidate bytes exceed expanded total", changed))

        for name, evidence in cases:
            with self.subTest(case=name):
                self.write_json(relative, evidence)
                self.assert_rejected_without_json()

    def test_probe_image_package_evidence_matches_locked_images_by_full_identity(self):
        result = self.generate()
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        manifest = json.loads(result.stdout)
        lock = self.read_json("compat.lock.json")
        evidence = self.read_json("docs/implementation/evidence/W-02-probe-image-packages.json")
        self.assertEqual(evidence["schema_version"], 1)
        self.assertEqual(evidence["status"], "partial")

        expected = {}
        for component in ("go", "llvm"):
            rows = [lock[component], *lock[component].get("additional_archives", [])]
            for row in rows:
                image = row.get("probe_image")
                if image:
                    expected[(image, row["platform"])] = row
        self.assertEqual(len(expected), 3)

        inventory_by_identity = {
            (item["image"], item["platform"]): item
            for item in evidence["inventories"]
        }
        self.assertEqual(len(evidence["inventories"]), 4)
        self.assertTrue(all(identity in inventory_by_identity for identity in expected))
        tool_inventory = next(item for item in evidence["inventories"] if item["role"] == "inventory_tool")
        self.assertNotIn((tool_inventory["image"], tool_inventory["platform"]), expected)
        locked_tool = lock["inventory_tool_image"]
        self.assertEqual(tool_inventory["image"], locked_tool["image"])
        self.assertEqual(tool_inventory["platform"], locked_tool["platform"])

        package_outputs = manifest["artifacts"]["probe_build_images"]
        actual = {
            (item["image"], item["platform"]): item
            for item in package_outputs
        }
        self.assertEqual(set(actual), set(expected))
        self.assertNotIn((tool_inventory["image"], tool_inventory["platform"]), actual)
        inventory_tool_outputs = [
            item for item in manifest["artifacts"].get("inventory_tool_images", [])
            if item.get("role") == "inventory_tool"
        ]
        self.assertEqual(len(inventory_tool_outputs), 1)
        self.assertEqual(inventory_tool_outputs[0]["image"], tool_inventory["image"])
        self.assertEqual(inventory_tool_outputs[0]["platform"], tool_inventory["platform"])
        self.assertIn("docs/implementation/evidence/W-02-probe-image-packages.json", {
            item["path"] for item in manifest["inputs"]
        })
        for relative in (
            "docs/implementation/evidence/W-02-probe-images-20261029/debian-index.json",
            "docs/implementation/evidence/W-02-probe-images-20261029/buildpack-index.json",
        ):
            self.assertIn(relative, {item["path"] for item in manifest["inputs"]})

        for identity, locked in expected.items():
            inventory = inventory_by_identity[identity]
            output = actual[identity]
            self.assertEqual(output["packages"], inventory["packages"])
            self.assertEqual(output["package_count"], inventory["package_count"])
            self.assertEqual(output["license_applicability"], "unknown")
            self.assertFalse(any(
                gap.get("scope") == f"probe image {identity[1]}"
                and gap.get("image") == identity[0]
                and gap.get("platform") == identity[1]
                and "no package or license inventory" in gap.get("reason", "")
                for gap in manifest["gaps"]
            ))

    def test_rejects_invalid_probe_image_package_evidence(self):
        relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        original = self.read_json(relative)
        cases = []

        changed = json.loads(json.dumps(original))
        changed["inventories"].append(dict(changed["inventories"][0]))
        cases.append(("duplicate inventory", changed))

        for field, value in (("image", "debian@sha256:" + "0" * 64), ("platform", "linux/arm64")):
            changed = json.loads(json.dumps(original))
            changed["inventories"][0][field] = value
            cases.append((f"mismatched {field}", changed))

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["package_count"] += 1
        cases.append(("package count mismatch", changed))

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["packages"][0]["copyright"]["sha256"] = "not-a-digest"
        cases.append(("invalid copyright digest", changed))

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["role"] = "unknown"
        cases.append(("unknown inventory role", changed))

        for name, evidence in cases:
            with self.subTest(case=name):
                self.write_json(relative, evidence)
                self.assert_rejected_without_json()

        path = self.repo / relative
        path.write_text('{"schema_version":1,"status":', encoding="utf-8")
        self.assert_rejected_without_json()

    def test_rejects_changed_probe_image_indexes_and_wrong_platform_descriptors(self):
        index_cases = (
            (
                "docs/implementation/evidence/W-02-probe-images-20261029/debian-index.json",
                "probe image index digest",
            ),
            (
                "docs/implementation/evidence/W-02-probe-images-20261029/buildpack-index.json",
                "probe image index digest",
            ),
        )
        for index, (relative, expected_error) in enumerate(index_cases):
            with self.subTest(index=relative):
                repo = Path(self.temp.name) / f"changed-index-{index}"
                self.make_repo(repo)
                path = repo / relative
                path.write_bytes(path.read_bytes() + b" ")
                result = self.generate(repo)
                self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
                self.assertEqual(result.stdout, b"")
                self.assertNotIn(b"Traceback", result.stderr)
                self.assertIn(expected_error.encode(), result.stderr)

        relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        repo = Path(self.temp.name) / "wrong-platform-descriptor"
        self.make_repo(repo)
        evidence = self.read_json(relative)
        changed = json.loads(json.dumps(evidence))
        arm64 = next(item for item in changed["inventories"] if item["role"] == "llvm_probe" and item["platform"] == "linux/arm64")
        amd64 = next(item for item in changed["inventories"] if item["role"] == "llvm_probe" and item["platform"] == "linux/amd64")
        arm64["image_manifest"], amd64["image_manifest"] = amd64["image_manifest"], arm64["image_manifest"]
        target = repo / relative
        target.write_text(json.dumps(changed), encoding="utf-8")
        result = self.generate(repo)
        self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        self.assertEqual(result.stdout, b"")
        self.assertNotIn(b"Traceback", result.stderr)
        self.assertIn(b"platform descriptor", result.stderr)

    def test_rejects_mutated_index_relabelled_as_its_own_direct_manifest(self):
        repo = Path(self.temp.name) / "mutated-index-as-manifest"
        self.make_repo(repo)
        index_relative = "docs/implementation/evidence/W-02-probe-images-20261029/debian-index.json"
        index_path = repo / index_relative
        modified = index_path.read_bytes() + b" "
        index_path.write_bytes(modified)
        modified_digest = hashlib.sha256(modified).hexdigest()

        evidence_relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        evidence_path = repo / evidence_relative
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        go_probe = next(item for item in evidence["inventories"] if item["role"] == "go_probe")
        go_probe["image_manifest"] = f"sha256:{modified_digest}"
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")

        result = self.generate(repo)
        self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        self.assertEqual(result.stdout, b"")
        self.assertNotIn(b"Traceback", result.stderr)
        self.assertIn(b"index", result.stderr.lower())

    def test_probe_package_copyright_gaps_keep_packages_without_fabricated_metadata(self):
        relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        original = self.read_json(relative)
        inventory = next(item for item in original["inventories"] if item["role"] == "go_probe")
        image, platform = inventory["image"], inventory["platform"]
        package_name = inventory["packages"][0]["package"]
        statuses = {
            "missing": {"path": "/usr/share/doc/apt/copyright", "status": "missing"},
            "unreadable": {
                "path": "/usr/share/doc/apt/copyright",
                "resolved_path": "/usr/share/doc/apt/copyright",
                "status": "unreadable",
            },
            "not_regular": {
                "path": "/usr/share/doc/apt/copyright",
                "resolved_path": "/usr/share/doc/apt/copyright",
                "status": "not_regular",
            },
            "outside_allowed_path": {
                "path": "/usr/share/doc/apt/copyright",
                "resolved_path": "/opt/licenses/apt/copyright",
                "status": "outside_allowed_path",
            },
            "unresolvable": {"path": "/usr/share/doc/apt/copyright", "status": "unresolvable"},
        }

        for status, copyright_record in statuses.items():
            with self.subTest(status=status):
                repo = Path(self.temp.name) / f"probe-copyright-{status}"
                self.make_repo(repo)
                changed = json.loads(json.dumps(original))
                target_inventory = next(item for item in changed["inventories"] if item["role"] == "go_probe")
                target_package = next(item for item in target_inventory["packages"] if item["package"] == package_name)
                target_package["copyright"] = copyright_record
                target_inventory["stdout_sha256"] = self.probe_stdout_digest(target_inventory["packages"])
                path = repo / relative
                path.write_text(json.dumps(changed), encoding="utf-8")

                result = self.generate(repo)
                self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
                manifest = json.loads(result.stdout)
                output = next(
                    item for item in manifest["artifacts"]["probe_build_images"]
                    if item["image"] == image and item["platform"] == platform
                )
                self.assertEqual(output["package_count"], inventory["package_count"])
                self.assertEqual(len(output["packages"]), inventory["package_count"])
                retained = next(item for item in output["packages"] if item["package"] == package_name)
                self.assertEqual(retained["version"], target_package["version"])
                self.assertEqual(retained["copyright"], copyright_record)
                self.assertNotIn("sha256", retained["copyright"])
                self.assertNotIn("size", retained["copyright"])
                self.assertTrue(any(
                    gap.get("image") == image
                    and gap.get("platform") == platform
                    and gap.get("package") == package_name
                    and gap.get("status") == status
                    and gap.get("path") == copyright_record["path"]
                    for gap in manifest["gaps"]
                ))

    def test_rejects_invalid_probe_package_copyright_statuses_and_shapes(self):
        relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        original = self.read_json(relative)
        cases = []

        changed = json.loads(json.dumps(original))
        changed["inventories"][0]["packages"][0]["copyright"].update({"status": "unknown_status"})
        cases.append(("unknown status", changed))

        changed = json.loads(json.dumps(original))
        copyright = changed["inventories"][0]["packages"][0]["copyright"]
        copyright["status"] = "missing"
        cases.append(("missing status with fabricated digest", changed))

        changed = json.loads(json.dumps(original))
        copyright = changed["inventories"][0]["packages"][0]["copyright"]
        copyright.pop("sha256")
        cases.append(("recorded status without digest", changed))

        for name, evidence in cases:
            with self.subTest(case=name):
                repo = Path(self.temp.name) / f"invalid-copyright-{name.replace(' ', '-') }"
                self.make_repo(repo)
                (repo / relative).write_text(json.dumps(evidence), encoding="utf-8")
                self.assert_rejected_without_json(repo)

    def test_rejects_probe_package_content_changed_without_stdout_digest_update(self):
        relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        original = self.read_json(relative)
        cases = []
        for field, value in (("version", "999.0"), ("sha256", "0" * 64)):
            changed = json.loads(json.dumps(original))
            package = changed["inventories"][0]["packages"][0]
            if field == "version":
                package[field] = value
            else:
                package["copyright"][field] = value
            cases.append((field, changed))

        for name, evidence in cases:
            with self.subTest(field=name):
                repo = Path(self.temp.name) / f"changed-probe-content-{name}"
                self.make_repo(repo)
                (repo / relative).write_text(json.dumps(evidence), encoding="utf-8")
                result = self.generate(repo)
                self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
                self.assertEqual(result.stdout, b"")
                self.assertNotIn(b"Traceback", result.stderr)
                self.assertIn(b"stdout", result.stderr.lower())

    def test_rejects_inventory_tool_identity_changed_with_matching_manifest_digest(self):
        relative = "docs/implementation/evidence/W-02-probe-image-packages.json"
        original = self.read_json(relative)
        for field, value in (
            ("image", "python@sha256:" + "0" * 64),
            ("platform", "linux/amd64"),
        ):
            with self.subTest(field=field):
                repo = Path(self.temp.name) / f"changed-tool-identity-{field}"
                self.make_repo(repo)
                changed = json.loads(json.dumps(original))
                tool_inventory = next(item for item in changed["inventories"] if item["role"] == "inventory_tool")
                if field == "image":
                    tool_inventory["image"] = value
                    tool_inventory["image_manifest"] = "sha256:" + "0" * 64
                else:
                    tool_inventory["platform"] = value
                (repo / relative).write_text(json.dumps(changed), encoding="utf-8")
                result = self.generate(repo)
                self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
                self.assertEqual(result.stdout, b"")
                self.assertNotIn(b"Traceback", result.stderr)
                self.assertIn(b"inventory tool", result.stderr.lower())

    def test_rejects_cross_source_archive_mismatch(self):
        php_path = "docs/implementation/evidence/W-02-php-license-files.json"
        php = self.read_json(php_path)
        php["archive_sha256"] = "0" * 64
        self.write_json(php_path, php)
        self.assert_rejected_without_json()

        lock = self.read_json("compat.lock.json")
        php["archive_sha256"] = lock["php"]["archive_sha256"]
        self.write_json(php_path, php)
        go_path = "docs/implementation/evidence/W-02-go-source-license-files.json"
        go = self.read_json(go_path)
        go["archive_sha256"] = "1" * 64
        self.write_json(go_path, go)
        self.assert_rejected_without_json()

        go["archive_sha256"] = lock["go"]["source_build"]["archive_sha256"]
        self.write_json(go_path, go)
        relative = "docs/implementation/evidence/W-02-build-image-packages.json"
        original_packages = self.read_json(relative)

        packages = json.loads(json.dumps(original_packages))
        packages["profiles"]["linux/amd64"]["image_manifest"] = "0" * 64
        self.write_json(relative, packages)
        self.assert_rejected_without_json()

        for profiles in (
            {key: value for key, value in original_packages["profiles"].items()
             if key != "linux/amd64"},
            {("linux/386" if key == "linux/amd64" else key): value
             for key, value in original_packages["profiles"].items()},
        ):
            packages = dict(original_packages)
            packages["profiles"] = profiles
            self.write_json(relative, packages)
            self.assert_rejected_without_json()

    def test_rejects_empty_duplicate_and_invalid_file_records(self):
        php_path = "docs/implementation/evidence/W-02-php-license-files.json"
        original = self.read_json(php_path)

        for files in ([], [original["files"][0], original["files"][0]]):
            with self.subTest(files=files):
                php = dict(original)
                php["files"] = files
                self.write_json(php_path, php)
                self.assert_rejected_without_json()

        for bad_record in (
            {**original["files"][0], "path": ""},
            {**original["files"][0], "sha256": "not-a-checksum"},
            {**original["files"][0], "sha256": None},
            {**original["files"][0], "size": True},
            {**original["files"][0], "size": None},
        ):
            with self.subTest(record=bad_record):
                php = dict(original)
                php["files"] = [bad_record]
                self.write_json(php_path, php)
                self.assert_rejected_without_json()

    def test_rejects_missing_null_duplicate_key_and_oversized_inputs(self):
        original = self.read_json("compat.lock.json")
        lock_path = self.repo / "compat.lock.json"

        lock_path.unlink()
        self.assert_rejected_without_json()
        self.write_json("compat.lock.json", None)
        self.assert_rejected_without_json()
        lock_path.write_text('{"schema_version":1,"schema_version":1}', encoding="utf-8")
        self.assert_rejected_without_json()
        lock_path.write_text(json.dumps(original), encoding="utf-8")

        php_path = self.repo / "docs/implementation/evidence/W-02-php-license-files.json"
        php_path.write_bytes(b" " * (10 * 1024 * 1024 + 1))
        self.assert_rejected_without_json()

    def test_rejects_symlinked_and_nonregular_fixed_inputs(self):
        cases = ("file", "directory", "nonregular")
        for case in cases:
            with self.subTest(case=case):
                repo = Path(self.temp.name) / f"symlink-{case}"
                self.make_repo(repo)
                outside = Path(self.temp.name) / f"outside-{case}"
                if case == "file":
                    target = repo / "compat.lock.json"
                    shutil.copyfile(target, outside)
                    target.unlink()
                    target.symlink_to(outside)
                elif case == "directory":
                    target = repo / "docs/implementation/evidence"
                    shutil.move(str(target), str(outside))
                    target.symlink_to(outside, target_is_directory=True)
                else:
                    target = repo / "compat.lock.json"
                    target.unlink()
                    target.mkdir()

                result = self.run_cli("generate", "--repo", str(repo))
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, b"")

    def test_rejects_oversized_check_manifest(self):
        manifest = self.repo / "too-large-manifest.json"
        manifest.write_bytes(b"x" * (10 * 1024 * 1024 + 1))
        checked = self.run_cli("check", "--repo", str(self.repo), str(manifest))
        self.assertNotEqual(checked.returncode, 0)
        self.assertEqual(checked.stdout, b"")

        manifest.unlink()
        manifest.mkdir()
        checked = self.run_cli("check", "--repo", str(self.repo), str(manifest))
        self.assertNotEqual(checked.returncode, 0)
        self.assertEqual(checked.stdout, b"")

    def test_rejects_package_profile_count_and_duplicate_packages(self):
        relative = "docs/implementation/evidence/W-02-build-image-packages.json"
        original = self.read_json(relative)

        for profile in original["profiles"].values():
            profile["package_count"] = True
        self.write_json(relative, original)
        self.assert_rejected_without_json()

        original = self.read_json(relative)
        for profile in original["profiles"].values():
            profile["packages"].append(dict(profile["packages"][0]))
            profile["package_count"] = len(profile["packages"])
        self.write_json(relative, original)
        self.assert_rejected_without_json()

    def test_rejects_non_string_encoding_and_probe_status_without_traceback(self):
        cases = (
            ("php encoding list", [], "files[0].encoding"),
            ("php encoding null", None, "files[0].encoding"),
            ("go primary probe object", {}, "compat.lock.json.go binary linux/arm64.probe_status"),
            ("go additional probe list", [], "compat.lock.json.go binary linux/amd64.probe_status"),
            ("llvm primary probe list", [], "compat.lock.json.llvm binary linux/arm64.probe_status"),
            ("llvm additional probe object", {}, "compat.lock.json.llvm binary linux/amd64.probe_status"),
        )
        for name, invalid_value, expected_error in cases:
            with self.subTest(case=name):
                repo = Path(self.temp.name) / f"invalid-{name.replace(' ', '-')}"
                self.make_repo(repo)
                if name.startswith("php"):
                    path = repo / "docs/implementation/evidence/W-02-php-license-files.json"
                    evidence = json.loads(path.read_text(encoding="utf-8"))
                    evidence["files"][0]["encoding"] = invalid_value
                    path.write_text(json.dumps(evidence), encoding="utf-8")
                else:
                    path = repo / "compat.lock.json"
                    lock = json.loads(path.read_text(encoding="utf-8"))
                    component = "go" if name.startswith("go") else "llvm"
                    if "additional" in name:
                        lock[component]["additional_archives"][0]["probe_status"] = invalid_value
                    else:
                        lock[component]["probe_status"] = invalid_value
                    path.write_text(json.dumps(lock), encoding="utf-8")

                result = self.assert_rejected_without_json(repo)
                self.assertIn(expected_error.encode(), result.stderr)

    def test_rejects_mismatched_go_source_build_version(self):
        lock = self.read_json("compat.lock.json")
        lock["go"]["source_build"]["version"] = "9.9.9"
        self.write_json("compat.lock.json", lock)
        self.assert_rejected_without_json()

    def test_rejects_known_official_go_and_llvm_platform_mismatches(self):
        cases = (
            ("go", "linux/386", "Go archive URL version/platform"),
            ("llvm", "linux/ppc64", "LLVM archive URL platform"),
        )
        for component, wrong_platform, expected_error in cases:
            with self.subTest(component=component):
                repo = Path(self.temp.name) / f"mismatched-{component}-archive"
                self.make_repo(repo)
                lock_path = repo / "compat.lock.json"
                lock = json.loads(lock_path.read_text(encoding="utf-8"))
                lock[component]["additional_archives"][0]["platform"] = wrong_platform
                lock_path.write_text(json.dumps(lock), encoding="utf-8")

                result = self.assert_rejected_without_json(repo)
                self.assertIn(expected_error.encode(), result.stderr)

    def test_failed_and_unknown_probe_statuses_remain_gaps(self):
        lock = self.read_json("compat.lock.json")
        lock["go"]["probe_status"] = "failed"
        lock["llvm"]["probe_status"] = "unknown"
        self.write_json("compat.lock.json", lock)

        result = self.generate()
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        manifest = json.loads(result.stdout)
        gaps = manifest["gaps"]
        self.assertTrue(any(
            gap.get("scope") == "Go binary probe linux/arm64"
            and gap.get("probe_status") == "failed"
            and gap.get("status") == "unknown"
            for gap in gaps
        ))
        self.assertTrue(any(
            gap.get("scope") == "LLVM binary probe linux/arm64"
            and gap.get("probe_status") == "unknown"
            and gap.get("status") == "unknown"
            for gap in gaps
        ))

    def test_records_non_utf8_candidates_as_gaps_and_keeps_future_platforms(self):
        go_path = "docs/implementation/evidence/W-02-go-source-license-files.json"
        go = self.read_json(go_path)
        non_utf8_path = "go/src/vendor/THIRD_PARTY_LICENSE"
        non_utf8_digest = "a" * 64
        go["files"].append(
            {
                "path": non_utf8_path,
                "size": 3,
                "sha256": non_utf8_digest,
                "encoding": "binary",
            }
        )
        self.write_json(go_path, go)

        lock = self.read_json("compat.lock.json")
        go_archive = {
            "platform": "freebsd/arm64",
            "archive_url": "https://example.invalid/go-freebsd-arm64.tar.gz",
            "archive_sha256": "b" * 64,
            "source_status": "downloaded_and_digest_verified",
            "probe_status": "not_run",
        }
        llvm_archive = {
            "platform": "linux/ppc64",
            "archive_url": "https://example.invalid/llvm-linux-ppc64.tar.zst",
            "archive_sha256": "c" * 64,
            "source_status": "downloaded_and_digest_verified",
        }
        lock["go"]["additional_archives"].append(go_archive)
        lock["llvm"].setdefault("additional_archives", []).append(llvm_archive)
        self.write_json("compat.lock.json", lock)

        result = self.generate()
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        manifest = json.loads(result.stdout)
        self.assertEqual(manifest.get("status"), "partial")
        gaps = json.dumps(manifest["gaps"], ensure_ascii=False, sort_keys=True).lower()
        all_output = json.dumps(manifest, ensure_ascii=False, sort_keys=True).lower()
        self.assertIn(non_utf8_path.lower(), gaps)
        candidate = next(gap for gap in manifest["gaps"] if gap.get("path") == non_utf8_path)
        self.assertEqual(candidate["size"], 3)
        self.assertEqual(candidate["sha256"], non_utf8_digest)
        self.assertEqual(candidate["encoding"], "binary")
        self.assertIn("freebsd/arm64", all_output)
        self.assertIn(go_archive["archive_sha256"], all_output)
        self.assertIn(llvm_archive["platform"], all_output)
        self.assertIn(llvm_archive["archive_sha256"], all_output)
        self.assertIn("freebsd/arm64", gaps)
        self.assertIn(llvm_archive["platform"], gaps)
        go_record = next(
            item for item in manifest["artifacts"]["go_binaries"]
            if item["platform"] == go_archive["platform"]
        )
        self.assertEqual(go_record["license_inventory_status"], "not_recorded")
        self.assertEqual(go_record["url_identity_status"], "unverified")
        self.assertTrue(any(
            gap.get("scope") == "Go binary archive identity freebsd/arm64"
            and gap.get("archive_url") == go_archive["archive_url"]
            and gap.get("status") == "unknown"
            for gap in manifest["gaps"]
        ))
        llvm_record = next(
            item for item in manifest["artifacts"]["llvm"]["archives"]
            if item["platform"] == llvm_archive["platform"]
        )
        self.assertEqual(llvm_record["license_inventory_status"], "not_recorded")
        self.assertEqual(llvm_record["url_identity_status"], "unverified")
        self.assertTrue(any(
            gap.get("scope") == "LLVM binary archive identity linux/ppc64"
            and gap.get("archive_url") == llvm_archive["archive_url"]
            and gap.get("status") == "unknown"
            for gap in manifest["gaps"]
        ))

    def test_allows_same_copyright_digest_at_distinct_paths(self):
        relative = "docs/implementation/evidence/W-02-build-image-packages.json"
        evidence = self.read_json(relative)
        profiles = list(evidence["profiles"].values())
        records = [record for profile in profiles for record in profile["packages"]]
        duplicate_digest = next(
            (record["copyright"]["sha256"] for record in records
             if sum(item["copyright"]["sha256"] == record["copyright"]["sha256"] for item in records) > 1),
            None,
        )
        self.assertIsNotNone(duplicate_digest, "fixture should contain shared copyright files")
        result = self.generate()
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))


if __name__ == "__main__":
    unittest.main()
