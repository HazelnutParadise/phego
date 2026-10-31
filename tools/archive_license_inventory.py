#!/usr/bin/env python3
"""Inventory filename-matched license candidates in a digest-verified archive.

Run untrusted archives only in a disposable, resource-limited container. This
utility does not establish license applicability or redistribution obligations.
"""
import argparse
import codecs
import gzip
import hashlib
import json
import os
import re
import stat
import struct
import sys
import tarfile
import tempfile
import zipfile
from pathlib import Path

from upstream_snapshot import _BoundedTarInfo

try:
    from compression.zstd import ZstdError
except ImportError:
    class ZstdError(Exception):
        pass

MAX_ARCHIVE = 2 * 1024**3
MAX_EXPANDED = 16 * 1024**3
MAX_CANDIDATE = 16 * 1024**2
MAX_CANDIDATES = 128 * 1024**2
MAX_ENTRIES = 100_000
MAX_CENTRAL_DIRECTORY = 64 * 1024**2
CHUNK = 1024 * 1024
PATTERN = re.compile(r'LICENSE|LICENCE|COPYING|NOTICE|PATENTS', re.I)
HEX = re.compile(r'[0-9a-f]{64}\Z')
GO_URL = re.compile(r'https://go\.dev/dl/go(?P<version>[0-9]+(?:\.[0-9]+){1,2})\.(?P<os>linux|darwin|windows)-(?P<arch>amd64|arm64)\.(?P<ext>tar\.gz|zip)\Z')
LLVM_URL = re.compile(r'https://github\.com/llvm/llvm-project/releases/download/llvmorg-(?P<version>[0-9]+(?:\.[0-9]+){1,2})/LLVM-(?P=version)-Linux-(?P<arch>ARM64|X64)\.tar\.zst\Z')


def unique_pairs(pairs):
    result = {}
    for k, v in pairs:
        if k in result:
            raise ValueError('duplicate JSON key')
        result[k] = v
    return result


def select(lock, component, platform):
    if not isinstance(lock, dict):
        raise ValueError('lock must be an object')
    obj = lock.get(component)
    if not isinstance(obj, dict) or not isinstance(obj.get('version'), str):
        raise ValueError('missing component and version')
    rows = [obj] + obj.get('additional_archives', [])
    if not all(isinstance(row, dict) for row in rows):
        raise ValueError('invalid archive records')
    matches = [row for row in rows if row.get('platform') == platform]
    if len(matches) != 1:
        raise ValueError('missing or ambiguous platform')
    row = matches[0]
    url, digest, version = row.get('archive_url'), row.get('archive_sha256'), obj['version']
    if not isinstance(url, str) or not isinstance(digest, str) or not HEX.fullmatch(digest):
        raise ValueError('invalid archive URL or checksum')
    if component == 'go':
        match = GO_URL.fullmatch(url)
        if not match or match['version'] != version or platform != match['os'] + '/' + match['arch']:
            raise ValueError('Go archive URL version/platform mismatch')
        if (match['os'] == 'windows') != (match['ext'] == 'zip'):
            raise ValueError('Go archive format/platform mismatch')
        return url, digest, version, 'go', match['ext']
    match = LLVM_URL.fullmatch(url)
    arch = {'ARM64': 'arm64', 'X64': 'amd64'}
    if not match or match['version'] != version or platform != 'linux/' + arch[match['arch']]:
        raise ValueError('LLVM archive URL version/platform mismatch')
    return url, digest, version, url.rsplit('/', 1)[-1][:-len('.tar.zst')], 'tar.zst'


def safe_path(name, root):
    if not isinstance(name, str) or not name or name.startswith('/') or '\\' in name:
        return False
    try:
        name.encode('utf-8', 'strict')
    except UnicodeError:
        return False
    return all(part not in ('', '.', '..') for part in name.split('/')) and (name == root or name.startswith(root + '/'))


def copy_and_verify(path, expected):
    temp = tempfile.TemporaryFile()
    digest = hashlib.sha256()
    size = 0
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
        with os.fdopen(fd, 'rb') as source:
            if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):
                raise ValueError('archive is not a regular file')
            while True:
                block = source.read(CHUNK)
                if not block:
                    break
                size += len(block)
                if size > MAX_ARCHIVE:
                    raise ValueError('compressed archive limit exceeded')
                temp.write(block)
                digest.update(block)
        if digest.hexdigest() != expected:
            raise ValueError('archive checksum mismatch')
        temp.seek(0)
        return temp, size
    except BaseException:
        temp.close()
        raise


def inspect_content(handle, size, candidate):
    if candidate and size > MAX_CANDIDATE:
        raise ValueError('candidate size limit exceeded')
    digest = hashlib.sha256()
    decoder = codecs.getincrementaldecoder('utf-8')('strict') if candidate else None
    utf8 = True
    remaining = size
    while remaining:
        block = handle.read(min(remaining, CHUNK))
        if not block:
            raise ValueError('truncated archive member')
        remaining -= len(block)
        if candidate:
            digest.update(block)
            if utf8:
                try:
                    decoder.decode(block)
                except UnicodeDecodeError:
                    utf8 = False
    if candidate and utf8:
        try:
            decoder.decode(b'', final=True)
        except UnicodeDecodeError:
            utf8 = False
    return digest.hexdigest(), utf8


def validate_zip_directory(f):
    f.seek(0, 2)
    size = f.tell()
    f.seek(max(0, size - 65557))
    tail = f.read()
    index = tail.rfind(b'PK\x05\x06')
    if index < 0 or len(tail) - index < 22:
        raise ValueError('missing ZIP end record')
    _, disk, cd_disk, n_disk, n_total, cd_size, cd_offset, comment = struct.unpack('<4s4H2IH', tail[index:index+22])
    if disk or cd_disk or n_disk != n_total or n_total == 65535 or cd_size == 0xffffffff or cd_offset == 0xffffffff:
        raise ValueError('multi-disk and ZIP64 archives are unsupported')
    if index + 22 + comment != len(tail) or n_total > MAX_ENTRIES or cd_size > MAX_CENTRAL_DIRECTORY or cd_offset + cd_size > size:
        raise ValueError('invalid or oversized ZIP directory')
    f.seek(0)


class LimitedReader:
    def __init__(self, source):
        self.source = source
        self.count = 0

    def read(self, size=-1):
        if size < 0:
            size = CHUNK
        block = self.source.read(min(size, MAX_EXPANDED - self.count + 1))
        self.count += len(block)
        if self.count > MAX_EXPANDED:
            raise ValueError('decompressed stream size limit exceeded')
        return block


def inventory(f, fmt, root):
    seen, candidates = set(), []
    count = expanded = candidate_bytes = 0
    def record(name, size, kind, reader):
        nonlocal count, expanded, candidate_bytes
        count += 1
        if count > MAX_ENTRIES:
            raise ValueError('archive member count limit exceeded')
        name = name.rstrip('/') if kind == 'directory' else name
        if not safe_path(name, root) or name in seen:
            raise ValueError('invalid or duplicate archive path')
        seen.add(name)
        candidate = bool(PATTERN.search(name.rsplit('/', 1)[-1]))
        if kind == 'directory':
            return
        if kind != 'file':
            if candidate:
                raise ValueError('candidate is not a regular file')
            return
        if size < 0:
            raise ValueError('invalid member size')
        expanded += size
        if expanded > MAX_EXPANDED:
            raise ValueError('archive expansion limit exceeded')
        if candidate:
            candidate_bytes += size
            if candidate_bytes > MAX_CANDIDATES:
                raise ValueError('candidate total size limit exceeded')
        digest, utf8 = inspect_content(reader, size, candidate)
        if candidate:
            candidates.append({'path': name, 'size': size, 'sha256': digest, 'encoding': 'utf8_text' if utf8 else 'binary'})
    if fmt == 'zip':
        validate_zip_directory(f)
        with zipfile.ZipFile(f) as archive:
            for member in archive.infolist():
                mode = member.external_attr >> 16
                file_type = stat.S_IFMT(mode)
                if file_type not in (0, stat.S_IFREG, stat.S_IFDIR, stat.S_IFLNK):
                    raise ValueError('unsupported ZIP member type')
                if member.is_dir() and file_type not in (0, stat.S_IFDIR):
                    raise ValueError('inconsistent ZIP directory type')
                if not member.is_dir() and file_type == stat.S_IFDIR:
                    raise ValueError('inconsistent ZIP member type')
                kind = 'directory' if member.is_dir() else ('link' if file_type == stat.S_IFLNK else 'file')
                if member.flag_bits & 1:
                    raise ValueError('encrypted ZIP entry')
                if kind == 'file':
                    with archive.open(member) as reader:
                        record(member.filename, member.file_size, kind, reader)
                        if reader.read(1):
                            raise ValueError("ZIP member exceeds declared size")
                else:
                    record(member.filename, member.file_size, kind, None)
    else:
        if fmt == 'tar.gz':
            stream = gzip.GzipFile(fileobj=f, mode='rb')
        else:
            try:
                from compression import zstd
            except ImportError as exc:
                raise ValueError('tar.zst requires Python 3.14 compression.zstd') from exc
            stream = zstd.ZstdFile(f, 'rb', options={zstd.DecompressionParameter.window_log_max: 30})
        with stream:
            limited = LimitedReader(stream)
            with tarfile.open(fileobj=limited, mode='r|', tarinfo=_BoundedTarInfo) as archive:
                for member in archive:
                    if member.isdir():
                        record(member.name, member.size, 'directory', None)
                    elif member.isfile():
                        reader = archive.extractfile(member)
                        if reader is None:
                            raise ValueError('cannot read archive member')
                        record(member.name, member.size, 'file', reader)
                    elif member.issym() or member.islnk():
                        record(member.name, member.size, 'link', None)
                    else:
                        raise ValueError('unsupported archive member type')
                # tarfile can prefetch beyond the end marker into its own buffer.
                if any(archive.fileobj.buf):
                    raise ValueError('non-zero data after tar end marker')
            # Consume the remaining stream to validate compression and reject appended tar files.
            while True:
                tail = limited.read(CHUNK)
                if not tail:
                    break
                if any(tail):
                    raise ValueError('non-zero data after tar end marker')
    if not candidates:
        raise ValueError('no regular license filename candidates')
    return sorted(candidates, key=lambda item: item['path']), count, expanded


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    capture = sub.add_parser('capture')
    capture.add_argument('--lock', required=True)
    capture.add_argument('--component', choices=('go', 'llvm'), required=True)
    capture.add_argument('--platform', required=True)
    capture.add_argument('--archive', required=True)
    args = parser.parse_args()
    try:
        lock_fd = os.open(args.lock, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
        with os.fdopen(lock_fd, 'rb') as lock_file:
            if not stat.S_ISREG(os.fstat(lock_file.fileno()).st_mode):
                raise ValueError('lock is not a regular file')
            raw = lock_file.read(10 * 1024 * 1024 + 1)
        if len(raw) > 10 * 1024 * 1024:
            raise ValueError('lock size limit exceeded')
        lock = json.loads(raw, object_pairs_hook=unique_pairs)
        url, sha, version, root, fmt = select(lock, args.component, args.platform)
        with copy_and_verify(args.archive, sha)[0] as verified:
            files, count, expanded = inventory(verified, fmt, root)
        result = {'schema_version': 1, 'status': 'partial', 'component': args.component, 'platform': args.platform,
                  'version': version, 'archive_url': url, 'archive_sha256': sha, 'format': fmt,
                  'selection': 'case-insensitive basename contains LICENSE, LICENCE, COPYING, NOTICE or PATENTS',
                  'license_applicability': 'unknown', 'entries': count, 'expanded_bytes': expanded, 'files': files}
        sys.stdout.write(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2) + '\n')
        return 0
    except (ValueError, OSError, EOFError, TypeError, KeyError, RecursionError, tarfile.TarError, zipfile.BadZipFile, ZstdError) as exc:
        print('archive_license_inventory: ' + str(exc), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
