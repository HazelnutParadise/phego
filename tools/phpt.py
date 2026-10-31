#!/usr/bin/env python3
"""Inspect PHPT tests without executing PHP or claiming product acceptance."""
import argparse
import hashlib
import json
import os
import stat
import sys

from phpt_runner import PHPTError, parse_phpt


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tools/phpt.py inspect tests/runner/phpt-fixtures/pass.phpt")
    sub = parser.add_subparsers(dest="operation")
    inspect = sub.add_parser("inspect", help="parse one bounded, regular PHPT file",
                             description="Parse a regular PHPT file up to 8 MiB; do not execute it.",
                             epilog="Example: python3 tools/phpt.py inspect tests/runner/phpt-fixtures/pass.phpt")
    inspect.add_argument("path", help="PHPT file to inspect")
    args = parser.parse_args()
    if args.operation is None:
        parser.print_help()
        return 0
    try:
        if not hasattr(os, "O_NONBLOCK") or not hasattr(os, "O_NOFOLLOW"):
            raise ValueError("safe file opening is unavailable on this platform")
        flags = os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW
        fd = os.open(args.path, flags)
        with os.fdopen(fd, "rb") as stream:
            metadata = os.fstat(stream.fileno())
            if not stat.S_ISREG(metadata.st_mode):
                raise ValueError("input must be a regular file")
            if metadata.st_size > 8 * 1024 * 1024:
                raise ValueError("input exceeds 8 MiB")
            data = stream.read(8 * 1024 * 1024 + 1)
            if len(data) > 8 * 1024 * 1024:
                raise ValueError("input exceeds 8 MiB")
        sections = parse_phpt(data, args.path)
        report = {"status":"parsed", "execution":"not_run", "product_acceptance":"not_claimed",
                  "sha256":hashlib.sha256(data).hexdigest(),
                  "sections":{name:len(value) for name, value in sections.items()}}
        print(json.dumps(report, sort_keys=True, indent=2))
        return 0
    except PHPTError as exc:
        message = str(exc)
        print(f"phpt: {message}", file=sys.stderr)
        if message.endswith("expected exactly one EXPECT section; found none"):
            print("  help: add one --EXPECT--, --EXPECTF--, or --EXPECTREGEX-- section", file=sys.stderr)
        return 2
    except (OSError, ValueError) as exc:
        print(f"phpt: {args.path}: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
