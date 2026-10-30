#!/usr/bin/env python3
"""Inspect and run PHPT tests in a restricted execution environment."""

import io
import re


class PHPTError(ValueError):
    pass

def parse_phpt(data: bytes, path: str) -> dict[str, bytes]:
    """Parse bounded PHPT sections, preserving their contents as bytes.

    Missing required sections, repeated markers, text before the first marker,
    and malformed markers produce a path-and-line error.
    """
    if len(data) > 8 * 1024 * 1024:
        raise PHPTError(f"{path}: PHPT input exceeds 8 MiB")
    sections: dict[str, bytes] = {}
    current = None
    chunks: list[bytes] = []
    marker = re.compile(rb"--([A-Z][A-Z0-9_]*)--")
    for line_number, line in enumerate(io.BytesIO(data), 1):
        # Retain strict full-line detection for unknown numeric section names,
        # and recognize PHP's letter/underscore markers with trailing text.
        match = marker.fullmatch(line.rstrip(b"\r\n")) or re.match(rb"--([_A-Z]+)--", line)
        if match:
            if current is not None:
                sections[current] = b"".join(chunks)
            name = match.group(1).decode("ascii")
            if current is None and name != "TEST":
                raise PHPTError(f"{path}:{line_number}: first section must be TEST")
            if name in sections:
                raise PHPTError(f"{path}:{line_number}: duplicate section {name}")
            current, chunks = name, []
        elif current is None:
            raise PHPTError(f"{path}:{line_number}: data before first PHPT section")
        else:
            chunks.append(line)
    if current is not None:
        sections[current] = b"".join(chunks)
    if "TEST" not in sections:
        raise PHPTError(f"{path}: missing TEST section")
    if "REDIRECTTEST" in sections:
        return sections
    for label, variants in (("FILE", ("FILE", "FILEEOF", "FILE_EXTERNAL")),
                            ("EXPECT", ("EXPECT", "EXPECTF", "EXPECTREGEX", "EXPECT_EXTERNAL", "EXPECTF_EXTERNAL", "EXPECTREGEX_EXTERNAL"))):
        present = [name for name in variants if name in sections]
        if label == "FILE" and "PHPDBG" in sections and not present:
            continue
        if len(present) != 1:
            raise PHPTError(f"{path}: expected exactly one {label} section; found {', '.join(present) or 'none'}")
    return sections
