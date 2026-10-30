"""Interpret PHPT INI settings for an explicit reference release."""

import re


_MAX_BYTES = 64 * 1024
_MAX_LINES = 256
_PHP_TRIM = b" \t\n\r\0\x0b"
_NAME = re.compile(rb"[A-Za-z0-9_.-]+\Z")
_FORBIDDEN_PLACEHOLDER = re.compile(rb"\{(?:TMP|ENV:[^}]*)\}|\{MAIL:[^}]*\}")


def parse_ini(data, release):
    """Return bounded, immutable PHP 8.5.12 INI settings."""
    if release != "8.5.12":
        raise ValueError("unsupported PHP release")
    if not isinstance(data, bytes):
        raise ValueError("INI input must be bytes")
    if len(data) > _MAX_BYTES:
        raise ValueError("INI input exceeds byte limit")
    if b"\0" in data:
        raise ValueError("INI input contains NUL byte")
    if b"$" in data or b"`" in data:
        raise ValueError("INI input contains shell expansion syntax")

    lines = re.split(rb"\r\n|[\r\n]", data)
    if len(lines) > _MAX_LINES:
        raise ValueError("INI input exceeds line limit")

    # Keep each key's first insertion slot; repeated extensions share a slot.
    keys = []
    normal_values = {}
    extension_values = {}
    for line in lines:
        if b"=" not in line:
            continue
        key, value = (part.strip(_PHP_TRIM) for part in line.split(b"=", 1))
        if not _NAME.fullmatch(key):
            raise ValueError("malformed INI setting name")
        if _FORBIDDEN_PLACEHOLDER.search(value):
            raise ValueError("unsupported INI substitution")
        value = value.replace(b"{PWD}", b"/work")
        setting = key + b"=" + value
        if key not in normal_values and key not in extension_values:
            keys.append(key)
        if key in (b"extension", b"zend_extension"):
            extension_values.setdefault(key, []).append(setting)
        else:
            normal_values[key] = setting

    entries = []
    for key in keys:
        if key in extension_values:
            entries.extend(extension_values[key])
        else:
            entries.append(normal_values[key])
    return tuple(entries)
