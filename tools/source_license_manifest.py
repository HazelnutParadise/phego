#!/usr/bin/env python3
"""Build and check a deterministic, explicitly partial source inventory."""

import argparse
import hashlib
import json
import os
import re
import stat
import sys
from pathlib import Path


SCHEMA_VERSION = 1
MAX_INPUT_BYTES = 10 * 1024 * 1024
INPUT_PATHS = (
    "compat.lock.json",
    "docs/implementation/evidence/W-02-php-license-files.json",
    "docs/implementation/evidence/W-02-go-source-license-files.json",
    "docs/implementation/evidence/W-02-build-image-packages.json",
)
SHA256_RE = re.compile(r"^[0-9a-fA-F]{64}$")
IMAGE_RE = re.compile(r"^([^@\s]+)@sha256:([0-9a-fA-F]{64})$")
GO_BINARY_ARCHIVE_RE = re.compile(
    r"^https://go\.dev/dl/go(?P<version>[0-9]+\.[0-9]+\.[0-9]+)\.(?P<os>[a-z0-9]+)-(?P<arch>[a-z0-9_]+)(?P<extension>\.tar\.gz|\.zip)$"
)
LLVM_ARCHIVE_RE = re.compile(
    r"^https://github\.com/llvm/llvm-project/releases/download/llvmorg-(?P<tag_version>[0-9]+\.[0-9]+\.[0-9]+)/LLVM-(?P<file_version>[0-9]+\.[0-9]+\.[0-9]+)-(?P<os>[A-Za-z0-9]+)-(?P<arch>[A-Za-z0-9]+)\.tar\.zst$"
)
LLVM_ARCHES = {"ARM64": "arm64", "X64": "amd64"}
COMPLETED_PROBE_STATUSES = frozenset(
    {
        "isolated_pure_go_build_passed",
        "isolated_pure_go_build_passed_under_emulation",
        "isolated_c_compile_and_lld_link_passed",
        "isolated_c_compile_and_lld_link_passed_under_emulation",
    }
)


class ManifestError(Exception):
    """An invalid or incomplete input prevents manifest generation."""


def _unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ManifestError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def _reject_json_constant(value):
    raise ManifestError(f"invalid JSON numeric constant: {value}")


def _read_limited_regular_file(path, relative, *, dir_fd=None):
    open_flags = os.O_RDONLY | getattr(os, "O_NONBLOCK", 0)
    try:
        if dir_fd is None:
            fd = os.open(path, open_flags)
        else:
            nofollow = getattr(os, "O_NOFOLLOW", None)
            if nofollow is None:
                raise ManifestError("platform does not support no-follow file opens")
            fd = os.open(path, open_flags | nofollow, dir_fd=dir_fd)
    except OSError as exc:
        raise ManifestError(f"cannot open {relative}: {exc.strerror or exc}") from exc
    try:
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            raise ManifestError(f"not a regular file: {relative}")
        remaining = MAX_INPUT_BYTES + 1
        chunks = []
        while remaining:
            chunk = os.read(fd, min(64 * 1024, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        raw = b"".join(chunks)
    finally:
        os.close(fd)
    if len(raw) > MAX_INPUT_BYTES:
        raise ManifestError(f"input exceeds {MAX_INPUT_BYTES} bytes: {relative}")
    return raw


def _read_repo_file(repo, relative):
    """Open a fixed repo-relative path without following any symlink component."""
    parts = Path(relative).parts
    if not parts or Path(relative).is_absolute() or any(part in {".", ".."} for part in parts):
        raise ManifestError(f"invalid fixed input path: {relative}")
    nofollow = getattr(os, "O_NOFOLLOW", None)
    directory = getattr(os, "O_DIRECTORY", None)
    if nofollow is None or directory is None:
        raise ManifestError("platform does not support safe repo-relative file opens")
    dir_flags = os.O_RDONLY | directory | nofollow
    try:
        current_fd = os.open(repo, dir_flags)
    except OSError as exc:
        raise ManifestError(f"cannot open repo directory: {exc.strerror or exc}") from exc
    try:
        for component in parts[:-1]:
            next_fd = os.open(component, dir_flags, dir_fd=current_fd)
            os.close(current_fd)
            current_fd = next_fd
        return _read_limited_regular_file(parts[-1], relative, dir_fd=current_fd)
    except OSError as exc:
        raise ManifestError(f"cannot open {relative}: {exc.strerror or exc}") from exc
    finally:
        os.close(current_fd)


def _read_json(repo, relative):
    raw = _read_repo_file(repo, relative)
    try:
        data = json.loads(
            raw.decode("utf-8"),
            object_pairs_hook=_unique_object,
            parse_constant=_reject_json_constant,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError) as exc:
        raise ManifestError(f"invalid JSON in {relative}: {exc}") from exc
    if not isinstance(data, dict):
        raise ManifestError(f"expected a JSON object in {relative}")
    return data, raw


def _required(obj, key, context):
    if key not in obj or obj[key] is None:
        raise ManifestError(f"missing or null {context}.{key}")
    return obj[key]


def _string(obj, key, context, nonempty=True):
    value = _required(obj, key, context)
    if not isinstance(value, str) or (nonempty and not value.strip()):
        raise ManifestError(f"{context}.{key} must be a {('non-empty ' if nonempty else '')}string")
    return value


def _optional_string(obj, key, context):
    if key not in obj or obj[key] is None:
        return None
    value = obj[key]
    if not isinstance(value, str) or not value.strip():
        raise ManifestError(f"{context}.{key} must be a non-empty string when present")
    return value


def _sha(value, context):
    if not isinstance(value, str) or not SHA256_RE.fullmatch(value):
        raise ManifestError(f"{context} must be a 64-character SHA-256 checksum")
    return value.lower()


def _sha_field(obj, key, context):
    return _sha(_required(obj, key, context), f"{context}.{key}")


def _list(obj, key, context, nonempty=True):
    value = _required(obj, key, context)
    if not isinstance(value, list) or (nonempty and not value):
        qualifier = "non-empty " if nonempty else ""
        raise ManifestError(f"{context}.{key} must be a {qualifier}array")
    return value


def _integer(obj, key, context, minimum=0):
    value = _required(obj, key, context)
    if type(value) is not int or value < minimum:
        raise ManifestError(f"{context}.{key} must be an integer >= {minimum}")
    return value


def _entries(records, context, *, require_encoding=False):
    if not isinstance(records, list) or not records:
        raise ManifestError(f"{context} must contain a non-empty array")
    seen = set()
    output = []
    unclassified = []
    for index, record in enumerate(records):
        label = f"{context}[{index}]"
        if not isinstance(record, dict):
            raise ManifestError(f"{label} must be an object")
        path = _string(record, "path", label)
        if "\x00" in path:
            raise ManifestError(f"{label}.path contains a NUL character")
        if path in seen:
            raise ManifestError(f"duplicate path in {context}: {path}")
        seen.add(path)
        size = _integer(record, "size", label)
        digest = _sha_field(record, "sha256", label)
        encoding = _string(record, "encoding", label) if "encoding" in record else None
        if require_encoding and encoding is None:
            raise ManifestError(f"{label}.encoding must be a non-empty string")
        entry = {"path": path, "size": size, "sha256": digest}
        if encoding is not None:
            entry["encoding"] = encoding
        if encoding is not None and encoding.lower() not in {"utf8_text", "utf-8", "utf8"}:
            unclassified.append(
                {
                    "path": path,
                    "size": size,
                    "sha256": digest,
                    "encoding": encoding,
                    "reason": "non-UTF-8 or unknown encoding; license applicability is unassessed",
                }
            )
        else:
            output.append(entry)
    return output, unclassified


def _image_digest(value, context):
    if not isinstance(value, str):
        raise ManifestError(f"{context} must be an image reference pinned by SHA-256")
    match = IMAGE_RE.fullmatch(value)
    if not match:
        raise ManifestError(f"{context} must be an image reference pinned by SHA-256")
    return match.group(2).lower()


def _locked_archives(lock, component, key, context, gaps):
    rows = []
    platforms = set()
    primary = component.get("platform")
    primary_row = {
        "platform": _string(component, "platform", context),
        "archive_url": _string(component, "archive_url", context),
        "archive_sha256": _sha_field(component, "archive_sha256", context),
    }
    if component.get("probe_image") is not None:
        probe_image = _string(component, "probe_image", context)
        _image_digest(probe_image, f"{context}.probe_image")
        primary_row["probe_image"] = probe_image
    platforms.add(primary_row["platform"])
    rows.append(primary_row)
    additional = component.get(key, [])
    if additional is None or not isinstance(additional, list):
        raise ManifestError(f"{context}.{key} must be an array when present")
    for index, archive in enumerate(additional):
        label = f"{context}.{key}[{index}]"
        if not isinstance(archive, dict):
            raise ManifestError(f"{label} must be an object")
        row = {
            "platform": _string(archive, "platform", label),
            "archive_url": _string(archive, "archive_url", label),
            "archive_sha256": _sha_field(archive, "archive_sha256", label),
        }
        if archive.get("probe_image") is not None:
            probe_image = _string(archive, "probe_image", label)
            _image_digest(probe_image, f"{label}.probe_image")
            row["probe_image"] = probe_image
        if row["platform"] in platforms:
            raise ManifestError(f"duplicate {context} platform: {row['platform']}")
        platforms.add(row["platform"])
        rows.append(row)
    return rows


def _record_archive_identity(archives, *, component_name, version, gaps):
    for archive in archives:
        platform = archive["platform"]
        url = archive["archive_url"]
        if component_name == "Go":
            match = GO_BINARY_ARCHIVE_RE.fullmatch(url)
            if match is None:
                archive["url_identity_status"] = "unverified"
                gaps.append(
                    {
                        "scope": f"Go binary archive identity {platform}",
                        "status": "unknown",
                        "platform": platform,
                        "archive_url": url,
                        "reason": "archive URL does not match the known official Go binary naming pattern",
                    }
                )
                continue
            archive_os = match.group("os")
            archive_arch = match.group("arch")
            archive_platform = f"{archive_os}/{archive_arch}"
            extension = match.group("extension")
            expected_extension = ".zip" if archive_os == "windows" else ".tar.gz"
            if match.group("version") != version or extension != expected_extension or archive_platform != platform:
                raise ManifestError(
                    f"Go archive URL version/platform does not match lock entry for {platform}: {url}"
                )
            archive["url_identity_status"] = "matches_known_official_pattern"
            continue

        match = LLVM_ARCHIVE_RE.fullmatch(url)
        if match is None:
            archive["url_identity_status"] = "unverified"
            gaps.append(
                {
                    "scope": f"LLVM binary archive identity {platform}",
                    "status": "unknown",
                    "platform": platform,
                    "archive_url": url,
                    "reason": "archive URL does not match the known official LLVM asset naming pattern",
                }
            )
            continue
        file_version = match.group("file_version")
        if match.group("tag_version") != version or file_version != version:
            raise ManifestError(f"LLVM archive URL version does not match lock entry for {platform}: {url}")
        normalized_os = match.group("os").lower()
        normalized_arch = LLVM_ARCHES.get(match.group("arch").upper())
        if normalized_arch is None:
            archive["url_identity_status"] = "unverified"
            gaps.append(
                {
                    "scope": f"LLVM binary archive identity {platform}",
                    "status": "unknown",
                    "platform": platform,
                    "archive_url": url,
                    "reason": "official LLVM asset architecture is not mapped by this inventory tool",
                }
            )
            continue
        if f"{normalized_os}/{normalized_arch}" != platform:
            raise ManifestError(
                f"LLVM archive URL platform does not match lock entry for {platform}: {url}"
            )
        archive["url_identity_status"] = "matches_known_official_pattern"


def _build(repo):
    inputs = {}
    raw_inputs = {}
    for relative in INPUT_PATHS:
        parsed, raw = _read_json(repo, relative)
        inputs[relative] = parsed
        raw_inputs[relative] = raw

    lock = inputs["compat.lock.json"]
    php_evidence = inputs[INPUT_PATHS[1]]
    go_evidence = inputs[INPUT_PATHS[2]]
    image_evidence = inputs[INPUT_PATHS[3]]

    if _integer(lock, "schema_version", "compat.lock.json", 1) != 1:
        raise ManifestError("unsupported compat.lock.json schema_version")
    if _string(lock, "status", "compat.lock.json") != "partial":
        raise ManifestError("compat.lock.json status must be partial")
    for name, evidence in (("PHP license evidence", php_evidence), ("Go source license evidence", go_evidence), ("build image evidence", image_evidence)):
        if _integer(evidence, "schema_version", name, 1) != 1:
            raise ManifestError(f"unsupported {name} schema_version")
        if _string(evidence, "status", name) != "partial":
            raise ManifestError(f"{name} status must be partial")

    php_lock = _required(lock, "php", "compat.lock.json")
    go_lock = _required(lock, "go", "compat.lock.json")
    llvm_lock = _required(lock, "llvm", "compat.lock.json")
    if not all(isinstance(item, dict) for item in (php_lock, go_lock, llvm_lock)):
        raise ManifestError("PHP, Go and LLVM lock entries must be objects")

    php_archive_sha = _sha_field(php_lock, "archive_sha256", "compat.lock.json.php")
    if _sha_field(php_evidence, "archive_sha256", "PHP license evidence") != php_archive_sha:
        raise ManifestError("PHP license inventory archive_sha256 does not match compat.lock.json")
    if _string(php_evidence, "archive_url", "PHP license evidence") != _string(
        php_lock, "archive_url", "compat.lock.json.php"
    ):
        raise ManifestError("PHP license inventory archive_url does not match compat.lock.json")
    _string(php_evidence, "selection", "PHP license evidence")
    _string(php_evidence, "limitations", "PHP license evidence")
    php_files, php_unclassified = _entries(
        _list(php_evidence, "files", "PHP license evidence"), "PHP license evidence.files"
    )

    go_source_lock = _required(go_lock, "source_build", "compat.lock.json.go")
    if not isinstance(go_source_lock, dict):
        raise ManifestError("compat.lock.json.go.source_build must be an object")
    go_source_sha = _sha_field(go_source_lock, "archive_sha256", "compat.lock.json.go.source_build")
    if _sha_field(go_evidence, "archive_sha256", "Go source license evidence") != go_source_sha:
        raise ManifestError("Go source license inventory archive_sha256 does not match compat.lock.json")
    go_version = _string(go_lock, "version", "compat.lock.json.go")
    go_source_version = _string(go_source_lock, "version", "compat.lock.json.go.source_build")
    if go_source_version != go_version:
        raise ManifestError("Go source_build.version does not match go.version")
    if _string(go_evidence, "archive_url", "Go source license evidence") != _string(
        go_source_lock, "archive_url", "compat.lock.json.go.source_build"
    ):
        raise ManifestError("Go source license inventory archive_url does not match compat.lock.json")
    _string(go_evidence, "selection", "Go source license evidence")
    for count_field in ("regular_files", "directories", "other_members"):
        _integer(go_evidence, count_field, "Go source license evidence")
    go_files, go_unclassified = _entries(
        _list(go_evidence, "files", "Go source license evidence"),
        "Go source license evidence.files",
        require_encoding=True,
    )

    gaps = []
    for note in _list(lock, "remaining", "compat.lock.json"):
        if not isinstance(note, str) or not note.strip():
            raise ManifestError("compat.lock.json.remaining entries must be non-empty strings")
        gaps.append({"scope": "project", "status": "unknown", "reason": note})
    if php_unclassified:
        gaps.extend({"scope": "PHP source", "status": "unknown", **item} for item in php_unclassified)
    if go_unclassified:
        gaps.extend({"scope": "Go source", "status": "unknown", **item} for item in go_unclassified)

    go_binary_archives = _locked_archives(lock, go_lock, "additional_archives", "compat.lock.json.go", gaps)
    llvm_archives = _locked_archives(lock, llvm_lock, "additional_archives", "compat.lock.json.llvm", gaps)
    go_binary_version = _string(go_lock, "version", "compat.lock.json.go")
    llvm_version = _string(llvm_lock, "version", "compat.lock.json.llvm")
    for archive in go_binary_archives:
        archive["version"] = go_binary_version
        archive["license_inventory_status"] = "not_recorded"
    for archive in llvm_archives:
        archive["version"] = llvm_version
        archive["license_inventory_status"] = "not_recorded"
    _record_archive_identity(
        go_binary_archives, component_name="Go", version=go_binary_version, gaps=gaps
    )
    _record_archive_identity(
        llvm_archives, component_name="LLVM", version=llvm_version, gaps=gaps
    )

    def archive_source(component, archive):
        if component.get("platform") == archive["platform"]:
            return component
        return next(
            row for row in component.get("additional_archives", [])
            if row.get("platform") == archive["platform"]
        )

    for component_name, component, archives in (
        ("Go binary", go_lock, go_binary_archives),
        ("LLVM binary", llvm_lock, llvm_archives),
    ):
        for archive in archives:
            source_record = archive_source(component, archive)
            source_context = f"compat.lock.json.{component_name.lower()} {archive['platform']}"
            probe_status = _optional_string(source_record, "probe_status", source_context)
            archive["probe_status"] = probe_status
            if probe_status not in COMPLETED_PROBE_STATUSES:
                gaps.append(
                    {
                        "scope": f"{component_name} probe {archive['platform']}",
                        "status": "unknown",
                        "probe_status": probe_status,
                        "reason": "probe status is not a recognized completed result",
                    }
                )
            gaps.append(
                {
                    "scope": f"{component_name} license inventory {archive['platform']}",
                    "status": "unknown",
                    "reason": "license file inventory for this binary archive is not recorded",
                }
            )
            if archive.get("probe_image"):
                gaps.append(
                    {
                        "scope": f"probe image {archive['platform']}",
                        "status": "unknown",
                        "image": archive["probe_image"],
                        "platform": archive["platform"],
                        "reason": "no package or license inventory is recorded for this probe image",
                    }
                )

    reference_profiles = {}
    primary_build = _required(lock, "reference_build", "compat.lock.json")
    if not isinstance(primary_build, dict):
        raise ManifestError("compat.lock.json.reference_build must be an object")
    platform = _string(primary_build, "platform", "compat.lock.json.reference_build")
    reference_profiles[platform] = {
        "image": _string(primary_build, "image", "compat.lock.json.reference_build"),
        "status": _string(primary_build, "status", "compat.lock.json.reference_build"),
    }
    additional_builds = lock.get("additional_reference_builds", [])
    if additional_builds is None or not isinstance(additional_builds, list):
        raise ManifestError("compat.lock.json.additional_reference_builds must be an array")
    for index, build in enumerate(additional_builds):
        context = f"compat.lock.json.additional_reference_builds[{index}]"
        if not isinstance(build, dict):
            raise ManifestError(f"{context} must be an object")
        build_platform = _string(build, "platform", context)
        if build_platform in reference_profiles:
            raise ManifestError(f"duplicate reference build platform: {build_platform}")
        reference_profiles[build_platform] = {
            "image": _string(build, "image", context),
            "status": _string(build, "status", context),
        }

    image_profiles = _required(image_evidence, "profiles", "build image evidence")
    if not isinstance(image_profiles, dict) or not image_profiles:
        raise ManifestError("build image evidence.profiles must be a non-empty object")
    _string(image_evidence, "scope", "build image evidence")
    _string(image_evidence, "limitations", "build image evidence")
    if set(image_profiles) != set(reference_profiles):
        raise ManifestError("build image profile platforms do not match compat.lock.json")
    index_digest = _sha(
        _string(image_evidence, "image_index", "build image evidence").removeprefix("sha256:"),
        "build image evidence.image_index",
    )
    if index_digest != _image_digest(reference_profiles[platform]["image"], "reference build image"):
        raise ManifestError("build image evidence.image_index does not match the locked reference image")

    image_records = []
    for profile_platform in sorted(image_profiles):
        profile = image_profiles[profile_platform]
        context = f"build image evidence.profiles.{profile_platform}"
        if not isinstance(profile, dict):
            raise ManifestError(f"{context} must be an object")
        manifest_value = _string(profile, "image_manifest", context)
        manifest_digest = _sha(manifest_value.removeprefix("sha256:"), f"{context}.image_manifest")
        expected_digest = _image_digest(reference_profiles[profile_platform]["image"], f"{context} lock image")
        if manifest_digest != expected_digest and not (
            profile_platform == platform and index_digest == expected_digest
        ):
            raise ManifestError(f"{context}.image_manifest does not match compat.lock.json")
        packages = _list(profile, "packages", context)
        count = _integer(profile, "package_count", context)
        if count != len(packages):
            raise ManifestError(f"{context}.package_count does not match packages length")
        seen_packages = set()
        package_rows = []
        seen_copyright_paths = set()
        for index, package_record in enumerate(packages):
            label = f"{context}.packages[{index}]"
            if not isinstance(package_record, dict):
                raise ManifestError(f"{label} must be an object")
            package_name = _string(package_record, "package", label)
            if package_name in seen_packages:
                raise ManifestError(f"duplicate package in {context}: {package_name}")
            seen_packages.add(package_name)
            version = _string(package_record, "version", label)
            copyright_data = _required(package_record, "copyright", label)
            if not isinstance(copyright_data, dict):
                raise ManifestError(f"{label}.copyright must be an object")
            copyright_path = _string(copyright_data, "path", f"{label}.copyright")
            if copyright_path in seen_copyright_paths:
                raise ManifestError(f"duplicate copyright path in {context}: {copyright_path}")
            seen_copyright_paths.add(copyright_path)
            package_rows.append(
                {
                    "package": package_name,
                    "version": version,
                    "copyright": {
                        "path": copyright_path,
                        "sha256": _sha_field(copyright_data, "sha256", f"{label}.copyright"),
                    },
                    "license_applicability": "unknown",
                }
            )
        image_records.append(
            {
                "platform": profile_platform,
                "image": reference_profiles[profile_platform]["image"],
                "image_index": _string(image_evidence, "image_index", "build image evidence"),
                "image_manifest": f"sha256:{manifest_digest}",
                "package_count": count,
                "packages": package_rows,
                "license_applicability": "unknown",
            }
        )
        if profile_platform == platform and manifest_digest != index_digest:
            gaps.append(
                {
                    "scope": f"reference image index mapping {profile_platform}",
                    "status": "unknown",
                    "image_index": _string(image_evidence, "image_index", "build image evidence"),
                    "image_manifest": f"sha256:{manifest_digest}",
                    "reason": "the index-to-platform-manifest correspondence has not been independently verified",
                }
            )
        if reference_profiles[profile_platform]["status"] != "complete":
            gaps.append(
                {
                    "scope": f"PHP reference build {profile_platform}",
                    "status": "unknown",
                    "reason": f"locked build status is {reference_profiles[profile_platform]['status']}",
                }
            )

    for scope in ("PHP source license applicability", "Go source license applicability", "LLVM license applicability", "build image package licenses"):
        gaps.append({"scope": scope, "status": "unknown", "reason": "license terms, applicability and redistribution obligations have not been reviewed"})

    input_rows = [
        {"path": relative, "sha256": hashlib.sha256(raw_inputs[relative]).hexdigest()}
        for relative in INPUT_PATHS
    ]
    return {
        "schema_version": SCHEMA_VERSION,
        "status": "partial",
        "inputs": input_rows,
        "artifacts": {
            "php_source": {
                "version": _string(php_lock, "version", "compat.lock.json.php"),
                "tag": _string(php_lock, "tag", "compat.lock.json.php"),
                "commit": _string(php_lock, "commit", "compat.lock.json.php"),
                "license": _string(php_lock, "license", "compat.lock.json.php"),
                "archive_url": _string(php_lock, "archive_url", "compat.lock.json.php"),
                "archive_sha256": php_archive_sha,
                "inventory_status": "partial",
                "filename_matches": php_files,
                "unclassified_candidates": php_unclassified,
                "license_applicability": "unknown",
            },
            "go_source": {
                "version": _string(go_source_lock, "version", "compat.lock.json.go.source_build"),
                "archive_url": _string(go_source_lock, "archive_url", "compat.lock.json.go.source_build"),
                "archive_sha256": go_source_sha,
                "license": _string(go_lock, "license", "compat.lock.json.go"),
                "inventory_status": "partial",
                "filename_matches": go_files,
                "unclassified_candidates": go_unclassified,
                "license_applicability": "unknown",
            },
            "go_binaries": go_binary_archives,
            "llvm": {
                "version": _string(llvm_lock, "version", "compat.lock.json.llvm"),
                "tag": _string(llvm_lock, "tag", "compat.lock.json.llvm"),
                "commit": _string(llvm_lock, "commit", "compat.lock.json.llvm"),
                "archives": llvm_archives,
                "license": _string(llvm_lock, "license", "compat.lock.json.llvm"),
                "license_source": _string(llvm_lock, "license_source", "compat.lock.json.llvm"),
                "inventory_status": "partial",
                "license_applicability": "unknown",
            },
            "reference_build_images": image_records,
        },
        "gaps": gaps,
    }


def _serialize(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False) + "\n").encode("utf-8")


def _repo_root(value):
    repo = Path(value).resolve()
    if not repo.is_dir():
        raise ManifestError(f"repo is not a directory: {value}")
    return repo


def main(argv=None):
    parser = argparse.ArgumentParser(description="Generate or check a partial source license manifest.")
    subparsers = parser.add_subparsers(dest="command", required=True)
    generate = subparsers.add_parser("generate", help="write deterministic JSON to stdout")
    generate.add_argument("--repo", required=True, help="repository containing the four fixed input files")
    check = subparsers.add_parser("check", help="compare a manifest byte-for-byte with regenerated JSON")
    check.add_argument("--repo", required=True, help="repository containing the four fixed input files")
    check.add_argument("manifest", help="manifest file to compare")
    args = parser.parse_args(argv)

    try:
        generated = _serialize(_build(_repo_root(args.repo)))
        if args.command == "generate":
            sys.stdout.buffer.write(generated)
            return 0
        manifest_path = Path(args.manifest)
        actual = _read_limited_regular_file(manifest_path, str(manifest_path))
        if actual != generated:
            raise ManifestError("manifest bytes differ from regenerated output")
        return 0
    except (ManifestError, OSError, ValueError) as exc:
        print(f"source_license_manifest: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
