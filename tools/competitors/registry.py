"""Freeze and independently verify a predeclared comparison plan.

Plan version 1 consists of competitors, workloads, platforms, modes and cells.
Each dimension is a nonempty list of unique nonempty string IDs.  Cells cover
their Cartesian product exactly once and contain competitor, workload, platform,
mode, status and evidence.  status is one of pending, comparable, unsupported.
Unsupported cells require nonempty evidence; pending cells remain blockers.
Files is a list of {path, sha256} entries, relative to the input root.
The tool must never execute files, follow symlinks, overwrite output, or treat
this structural check as proof of completeness or performance.
"""
import argparse
import hashlib
import itertools
import json
import os
import re
import stat
import struct
from dataclasses import dataclass
from pathlib import Path


MAX_PLAN_BYTES = 1024 * 1024
MAX_FILE_BYTES = 64 * 1024 * 1024
MAX_FILES = 4096
MAX_TOTAL_BYTES = 256 * 1024 * 1024
MAX_CELLS = 100_000
DIMENSIONS = ("competitors", "workloads", "platforms", "modes")
CELL_FIELDS = {"competitor", "workload", "platform", "mode", "status", "evidence"}
PLAN_FIELDS = {"version", *DIMENSIONS, "cells", "files"}
HEX_DIGEST = re.compile(r"[0-9a-f]{64}\Z")
ARCHIVE_MAGIC = b"PHEGO-REGISTRY\x01\x00"
ARCHIVE_HEADER_SIZE = len(ARCHIVE_MAGIC) + 8 + 32
MAX_ARCHIVE_BYTES = MAX_TOTAL_BYTES + MAX_PLAN_BYTES + MAX_FILES * 8 + ARCHIVE_HEADER_SIZE + 8
OPEN_SUPPORTS_DIR_FD = os.open in os.supports_dir_fd


@dataclass(frozen=True)
class Snapshot:
    plan_json: bytes
    sources: tuple
    blockers: int


def _fail(field, reason):
    raise ValueError(f"{field}: {reason}")


def _pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate key")
        result[key] = value
    return result


def _constant(_value):
    raise ValueError("non-finite number")


def _json(data, field):
    try:
        return json.loads(data.decode("utf-8"), object_pairs_hook=_pairs, parse_constant=_constant)
    except (UnicodeError, ValueError, RecursionError):
        _fail(field, "invalid JSON")


def _absolute(path, field):
    try:
        raw = os.fspath(path)
        if not isinstance(raw, str) or "\x00" in raw:
            _fail(field, "invalid path")
        result = Path(os.path.abspath(raw))
    except (TypeError, ValueError, OSError):
        _fail(field, "invalid path")
    # macOS exposes /tmp and /var as fixed system aliases. Normalize only these
    # well-known aliases before checking every component of the actual path.
    for alias in ("/tmp", "/var"):
        try:
            if str(result) == alias or str(result).startswith(alias + "/"):
                alias_path = Path(alias)
                if alias_path.is_symlink() and os.path.realpath(alias) in ("/private/tmp", "/private/var"):
                    result = Path(os.path.realpath(alias)) / str(result)[len(alias):].lstrip("/")
                break
        except OSError:
            _fail(field, "path cannot be inspected")
    current = Path(result.anchor)
    for part in result.parts[1:]:
        current /= part
        try:
            mode = current.lstat().st_mode
        except FileNotFoundError:
            continue
        except OSError:
            _fail(field, "path cannot be inspected")
        if stat.S_ISLNK(mode):
            _fail(field, "symlink path component is not allowed")
    return result


def _read(path, field, limit):
    """Open every path component relative to a no-follow directory descriptor."""
    absolute = _absolute(path, field)
    parent_fd = _open_directory(absolute.parent, field)
    try:
        return _read_at(parent_fd, absolute.name, field, limit)
    finally:
        os.close(parent_fd)


def _secure_flags():
    if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_DIRECTORY") or not OPEN_SUPPORTS_DIR_FD:
        _fail("filesystem", "secure no-follow directory opening is unavailable")
    return os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW


def _open_directory(path, field):
    """Open an absolute directory without following any path component."""
    flags = _secure_flags()
    try:
        absolute = _absolute(path, field)
        fd = os.open(absolute.anchor, flags)
        try:
            for component in absolute.parts[1:]:
                next_fd = os.open(component, flags, dir_fd=fd)
                os.close(fd)
                fd = next_fd
            if not stat.S_ISDIR(os.fstat(fd).st_mode):
                _fail(field, "must be a directory")
            return fd
        except Exception:
            os.close(fd)
            raise
    except ValueError:
        raise
    except (OSError, TypeError, NotImplementedError):
        _fail(field, "directory cannot be opened safely")


def _read_at(parent_fd, name, field, limit):
    _secure_flags()
    flags = os.O_RDONLY | os.O_NOFOLLOW | getattr(os, "O_NONBLOCK", 0)
    fd = None
    try:
        fd = os.open(name, flags, dir_fd=parent_fd)
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode):
            _fail(field, "must be a regular file")
        stream = os.fdopen(fd, "rb")
        fd = None
        with stream:
            data = stream.read(limit + 1)
        if len(data) > limit:
            _fail(field, "file size limit exceeded")
        return data
    except ValueError:
        raise
    except (OSError, TypeError, NotImplementedError):
        _fail(field, "file cannot be read safely")
    finally:
        if fd is not None:
            os.close(fd)


def _plan(plan):
    if isinstance(plan, dict):
        try:
            encoded = json.dumps(plan, ensure_ascii=False, allow_nan=False,
                                 separators=(",", ":")).encode("utf-8")
        except (TypeError, ValueError, UnicodeError, RecursionError):
            _fail("plan", "must contain valid JSON values")
        if len(encoded) > MAX_PLAN_BYTES:
            _fail("plan", "size limit exceeded")
        return _json(encoded, "plan")
    path = _absolute(plan, "plan")
    return _json(_read(path, "plan", MAX_PLAN_BYTES), "plan")


def _relative(value):
    if not isinstance(value, str) or not value or "\x00" in value or "\\" in value:
        _fail("files.path", "must be a safe relative path")
    if value.startswith("/") or re.match(r"^[A-Za-z]:", value):
        _fail("files.path", "must be a safe relative path")
    parts = value.split("/")
    if len(parts) > 128 or any(part in ("", ".", "..") for part in parts):
        _fail("files.path", "must be a safe relative path")
    return value


def _validate(root, obj, source_override=None):
    if not isinstance(obj, dict) or set(obj) != PLAN_FIELDS:
        _fail("plan", "fields do not match version 1")
    if type(obj["version"]) is not int or obj["version"] != 1:
        _fail("version", "must be 1")
    dims, product = {}, 1
    for name in DIMENSIONS:
        values = obj[name]
        if not isinstance(values, list) or not values:
            _fail(name, "must be a nonempty list")
        if len(values) > MAX_CELLS:
            _fail(name, "item limit exceeded")
        seen = set()
        for value in values:
            if not isinstance(value, str) or not value or value.strip() != value:
                _fail(name, "items must be nonempty strings")
            if value in seen:
                _fail(name, "duplicate item")
            seen.add(value)
        dims[name] = values
        product *= len(values)
        if product > MAX_CELLS:
            _fail("cells", "Cartesian product limit exceeded")
    cells = obj["cells"]
    if not isinstance(cells, list) or len(cells) != product:
        _fail("cells", "must cover the Cartesian product exactly once")
    expected = set(itertools.product(*(dims[name] for name in DIMENSIONS)))
    actual, blockers = set(), 0
    for cell in cells:
        if not isinstance(cell, dict) or set(cell) != CELL_FIELDS:
            _fail("cells", "cell fields do not match version 1")
        key = tuple(cell[name] for name in ("competitor", "workload", "platform", "mode"))
        if not all(isinstance(value, str) for value in key) or key not in expected:
            _fail("cells", "contains an unknown dimension ID")
        if key in actual:
            _fail("cells", "contains a duplicate cell")
        actual.add(key)
        status, evidence = cell["status"], cell["evidence"]
        if not isinstance(status, str) or status not in ("pending", "comparable", "unsupported"):
            _fail("cells.status", "must be pending, comparable, or unsupported")
        if not isinstance(evidence, str):
            _fail("cells.evidence", "must be a string")
        if status == "unsupported" and not evidence.strip():
            _fail("cells.evidence", "unsupported cells require evidence")
        if status != "comparable":
            blockers += 1
    if actual != expected:
        _fail("cells", "must cover the Cartesian product exactly once")
    entries = obj["files"]
    if not isinstance(entries, list) or not entries or len(entries) > MAX_FILES:
        _fail("files", "must be a nonempty bounded list")
    parsed, paths = [], set()
    for entry in entries:
        if not isinstance(entry, dict) or set(entry) != {"path", "sha256"}:
            _fail("files", "entries must contain path and sha256")
        rel, digest = _relative(entry["path"]), entry["sha256"]
        if rel in paths:
            _fail("files.path", "duplicate path")
        paths.add(rel)
        if not isinstance(digest, str) or not HEX_DIGEST.fullmatch(digest):
            _fail("files.sha256", "must be a lowercase SHA-256 digest")
        parsed.append((rel, digest))
    for rel in paths:
        parts = rel.split("/")
        if any("/".join(parts[:index]) in paths for index in range(1, len(parts))):
            _fail("files.path", "file paths cannot contain another file")
    source_bytes, total = [], 0
    if source_override is None:
        base = _absolute(root, "root")
        base_fd = _open_directory(base, "root")
        try:
            for rel, expected_digest in parsed:
                current_fd = os.dup(base_fd)
                try:
                    parts = rel.split("/")
                    for component in parts[:-1]:
                        next_fd = os.open(component, _secure_flags(), dir_fd=current_fd)
                        os.close(current_fd)
                        current_fd = next_fd
                    data = _read_at(current_fd, parts[-1], "files.path", MAX_FILE_BYTES)
                except ValueError:
                    raise
                except (OSError, TypeError, NotImplementedError):
                    _fail("files.path", "source is missing or contains a symlink")
                finally:
                    os.close(current_fd)
                total += len(data)
                if total > MAX_TOTAL_BYTES:
                    _fail("files", "total size limit exceeded")
                if hashlib.sha256(data).hexdigest() != expected_digest:
                    _fail("files.sha256", "source digest does not match")
                source_bytes.append((rel, data, expected_digest))
        finally:
            os.close(base_fd)
    else:
        if not isinstance(source_override, dict) or set(source_override) != {rel for rel, _ in parsed}:
            _fail("files", "archive source inventory does not match plan")
        for rel, expected_digest in parsed:
            data = source_override[rel]
            if not isinstance(data, bytes) or len(data) > MAX_FILE_BYTES:
                _fail("files", "archive source size limit exceeded")
            total += len(data)
            if total > MAX_TOTAL_BYTES:
                _fail("files", "total size limit exceeded")
            if hashlib.sha256(data).hexdigest() != expected_digest:
                _fail("files.sha256", "source digest does not match")
            source_bytes.append((rel, data, expected_digest))
    canonical = json.dumps(obj, ensure_ascii=False, sort_keys=True,
                           separators=(",", ":"), allow_nan=False).encode("utf-8")
    return Snapshot(canonical, tuple(source_bytes), blockers)


def validate(root, plan):
    """Return a validated immutable snapshot, raising field-specific errors."""
    return _validate(root, _plan(plan))


def _archive_parts(snapshot):
    plan = json.loads(snapshot.plan_json.decode("utf-8"))
    metadata = json.dumps({"format": 1, "plan": plan}, ensure_ascii=False,
                          sort_keys=True, separators=(",", ":"), allow_nan=False).encode("utf-8")
    if len(metadata) > MAX_PLAN_BYTES + 128:
        _fail("plan", "archive metadata size limit exceeded")
    metadata_length = struct.pack(">Q", len(metadata))
    digest = hashlib.sha256()
    digest.update(metadata_length)
    digest.update(metadata)
    file_parts = []
    payload_length = len(metadata_length) + len(metadata)
    for _rel, data, _expected_digest in snapshot.sources:
        length = struct.pack(">Q", len(data))
        digest.update(length)
        digest.update(data)
        file_parts.append((length, data))
        payload_length += len(length) + len(data)
    if payload_length + ARCHIVE_HEADER_SIZE > MAX_ARCHIVE_BYTES:
        _fail("output", "archive size limit exceeded")
    header = ARCHIVE_MAGIC + struct.pack(">Q", payload_length) + digest.digest()
    return header, metadata_length, metadata, tuple(file_parts)


def freeze(root, plan, output):
    """Write a self-contained archive with exclusive creation.

    Failed writes leave the partial file in place. This does not prevent a
    same-UID process from modifying the opened inode or renaming the output
    path after return.
    """
    snapshot = validate(root, plan)
    header, metadata_length, metadata, file_parts = _archive_parts(snapshot)
    dest = _absolute(output, "output")
    parent_fd = _open_directory(dest.parent, "output")
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | getattr(os, "O_NONBLOCK", 0)
    try:
        fd = os.open(dest.name, flags, 0o600, dir_fd=parent_fd)
    except FileExistsError:
        os.close(parent_fd)
        _fail("output", "destination already exists")
    except OSError:
        os.close(parent_fd)
        _fail("output", "archive cannot be created safely")
    fd_owned = True
    try:
        info = os.fstat(fd)
        identity = (info.st_dev, info.st_ino)
        if not stat.S_ISREG(info.st_mode):
            _fail("output", "created archive is not a regular file")
        try:
            stream = os.fdopen(fd, "wb", buffering=0)
            fd_owned = False
            with stream:
                chunks = (header, metadata_length, metadata) + tuple(
                    chunk for size, data in file_parts for chunk in (size, data)
                )
                for chunk in chunks:
                    written = stream.write(chunk)
                    if written != len(chunk):
                        _fail("output", "archive write was incomplete")
                stream.flush()
                os.fsync(stream.fileno())
                final_info = os.fstat(stream.fileno())
                if (final_info.st_dev, final_info.st_ino) != identity:
                    _fail("output", "archive descriptor identity changed")
        except ValueError:
            raise
        except OSError:
            _fail("output", "archive write failed; partial file was retained")
        try:
            named = os.stat(dest.name, dir_fd=parent_fd, follow_symlinks=False)
        except OSError:
            _fail("output", "archive name changed during writing")
        if not stat.S_ISREG(named.st_mode) or (named.st_dev, named.st_ino) != identity:
            _fail("output", "archive name changed during writing")
    except ValueError:
        raise
    except OSError:
        _fail("output", "archive operation failed; partial file was retained")
    finally:
        if fd_owned:
            os.close(fd)
        os.close(parent_fd)
    return snapshot.blockers


def _read_exact(stream, length, field):
    blocks = []
    remaining = length
    while remaining:
        block = stream.read(min(remaining, 1024 * 1024))
        if not block:
            _fail(field, "archive is truncated")
        blocks.append(block)
        remaining -= len(block)
    return b"".join(blocks)


def verify(bundle):
    """Verify a single-file archive directly, without extracting its contents."""
    path = _absolute(bundle, "bundle")
    parent_fd = _open_directory(path.parent, "bundle")
    try:
        flags = os.O_RDONLY | os.O_NOFOLLOW | getattr(os, "O_NONBLOCK", 0)
        try:
            fd = os.open(path.name, flags, dir_fd=parent_fd)
        except OSError:
            _fail("bundle", "archive file is missing or unsafe")
        fd_owned = True
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode):
                _fail("bundle", "archive must be a regular file")
            if info.st_size < ARCHIVE_HEADER_SIZE + 8 or info.st_size > MAX_ARCHIVE_BYTES:
                _fail("bundle", "archive size is outside the allowed limit")
            stream = os.fdopen(fd, "rb")
            fd_owned = False
            with stream:
                fixed = _read_exact(stream, ARCHIVE_HEADER_SIZE, "bundle")
                if not fixed.startswith(ARCHIVE_MAGIC):
                    _fail("bundle", "archive format is invalid")
                offset = len(ARCHIVE_MAGIC)
                payload_length = struct.unpack(">Q", fixed[offset:offset + 8])[0]
                expected_hash = fixed[offset + 8:offset + 40]
                if payload_length + ARCHIVE_HEADER_SIZE != info.st_size:
                    _fail("bundle", "archive length does not match its contents")
                if payload_length > MAX_ARCHIVE_BYTES - ARCHIVE_HEADER_SIZE:
                    _fail("bundle", "archive size limit exceeded")
                digest = hashlib.sha256()
                consumed = 0

                def take(length, field):
                    nonlocal consumed
                    if length < 0 or consumed + length > payload_length:
                        _fail(field, "archive record exceeds declared length")
                    block = _read_exact(stream, length, field)
                    digest.update(block)
                    consumed += length
                    return block

                metadata_length_bytes = take(8, "manifest")
                metadata_length = struct.unpack(">Q", metadata_length_bytes)[0]
                if metadata_length > MAX_PLAN_BYTES + 128:
                    _fail("manifest", "metadata size limit exceeded")
                metadata_bytes = take(metadata_length, "manifest")
                metadata = _json(metadata_bytes, "manifest")
                if not isinstance(metadata, dict) or set(metadata) != {"format", "plan"}:
                    _fail("manifest", "fields do not match archive format")
                if type(metadata["format"]) is not int or metadata["format"] != 1:
                    _fail("manifest", "format version is invalid")
                plan = metadata["plan"]
                if not isinstance(plan, dict) or set(plan) != PLAN_FIELDS:
                    _fail("plan", "fields do not match version 1")
                entries = plan["files"]
                if not isinstance(entries, list) or not entries or len(entries) > MAX_FILES:
                    _fail("files", "must be a nonempty bounded list")
                paths = set()
                for entry in entries:
                    if not isinstance(entry, dict) or set(entry) != {"path", "sha256"}:
                        _fail("files", "entries must contain path and sha256")
                    rel, sha = _relative(entry["path"]), entry["sha256"]
                    if rel in paths:
                        _fail("files.path", "duplicate path")
                    paths.add(rel)
                    if not isinstance(sha, str) or not HEX_DIGEST.fullmatch(sha):
                        _fail("files.sha256", "must be a lowercase SHA-256 digest")
                sources, total = {}, 0
                for entry in entries:
                    size_bytes = take(8, "files")
                    size = struct.unpack(">Q", size_bytes)[0]
                    if size > MAX_FILE_BYTES:
                        _fail("files", "source size limit exceeded")
                    total += size
                    if total > MAX_TOTAL_BYTES:
                        _fail("files", "total size limit exceeded")
                    sources[entry["path"]] = take(size, "files")
                if consumed != payload_length:
                    _fail("bundle", "archive has trailing content")
                if stream.read(1):
                    _fail("bundle", "archive has trailing content")
                if digest.digest() != expected_hash:
                    _fail("bundle", "archive integrity check failed")
                return _validate(None, plan, source_override=sources).blockers
        except ValueError:
            raise
        except (OSError, TypeError, NotImplementedError):
            _fail("bundle", "archive verification failed safely")
        finally:
            if fd_owned:
                os.close(fd)
    finally:
        os.close(parent_fd)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    v = sub.add_parser("validate"); v.add_argument("--root", required=True); v.add_argument("--plan", required=True)
    f = sub.add_parser("freeze"); f.add_argument("--root", required=True); f.add_argument("--plan", required=True); f.add_argument("--output", required=True)
    c = sub.add_parser("verify"); c.add_argument("--bundle", required=True)
    args = parser.parse_args(argv)
    try:
        if args.command == "validate":
            result = validate(args.root, args.plan)
            values = {"status": "valid", "files": len(result.sources), "blockers": result.blockers}
        elif args.command == "freeze":
            values = {"status": "frozen", "blockers": freeze(args.root, args.plan, args.output)}
        else:
            values = {"status": "verified", "blockers": verify(args.bundle)}
        values["product_acceptance"] = "not established"
        values["comparison"] = "not run"
        print(json.dumps(values, sort_keys=True))
    except ValueError:
        raise


if __name__ == "__main__":
    try:
        main()
    except ValueError as exc:
        raise SystemExit(f"error: {exc}") from None
