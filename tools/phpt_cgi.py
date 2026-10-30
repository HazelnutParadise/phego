"""Bounded PHP 8.5.12 PHPT request and CGI response handling."""
import math
import re
from urllib.parse import unquote_to_bytes

_TRIM = b" \t\n\r\x00\x0b"
_NUMERIC = re.compile(rb"[+-]?(?:(?:[0-9]+(?:\.[0-9]*)?)|(?:\.[0-9]+))(?:[eE][+-]?[0-9]+)?\Z")
_INTEGER = re.compile(rb"[+-]?[0-9]+\Z")
_REQUEST = ("CGI", "GET", "POST", "COOKIE", "EXPECTHEADERS")


def requested(sections):
    return any(name in sections for name in _REQUEST)


def needs_cgi(sections):
    return "CGI" in sections or any(sections.get(k, b"") not in (b"", b"0")
                                    for k in ("GET", "POST", "COOKIE", "EXPECTHEADERS"))


def request(sections):
    """Build a request from already byte-validated PHPT sections."""
    query = sections.get("GET", b"").strip(_TRIM)
    cookie = sections.get("COOKIE", b"").strip(_TRIM)
    post_section = sections.get("POST", b"")
    post = post_section.strip(_TRIM) if post_section not in (b"", b"0") else None
    if len(query) > 65536 or len(cookie) > 65536:
        raise ValueError("CGI query or cookie exceeds 64 KiB")
    if b"\0" in query or b"\0" in cookie:
        raise ValueError("CGI query or cookie contains NUL")
    if post is not None and len(post_section) > 1024 * 1024:
        raise ValueError("CGI POST exceeds 1 MiB")
    if b"=" not in query and unquote_to_bytes(query.replace(b"+", b" ")).lstrip(bytes(range(33))).startswith(b"-"):
        raise ValueError("CGI query would disable command options")
    return query, cookie, post


def _numeric_equal(left, right, bits):
    if left == right:
        return True
    left, right = left.strip(b" \t\n\r\v\f"), right.strip(b" \t\n\r\v\f")
    if not (_NUMERIC.fullmatch(left) and _NUMERIC.fullmatch(right)):
        return False
    if len(left) > 4096 or len(right) > 4096:
        return None
    try:
        lval, rval = float(left), float(right)
    except (ValueError, OverflowError):
        return None
    if math.isinf(lval) or math.isinf(rval):
        return False if lval == rval else lval == rval
    if _INTEGER.fullmatch(left) and _INTEGER.fullmatch(right):
        def bounded(value):
            negative = value.startswith(b"-")
            digits = value.lstrip(b"+-").lstrip(b"0") or b"0"
            if len(digits) > 20:
                return None
            number = int(digits)
            number = -number if negative else number
            return number if -(1 << (bits - 1)) <= number < (1 << (bits - 1)) else None
        left_int, right_int = bounded(left), bounded(right)
        if left_int is not None and right_int is not None:
            return left_int == right_int
        if (left_int is None) != (right_int is None):
            return False
        # Zend uses byte comparison when both overflow and float equality
        # would lose integer precision.
        if lval == rval and (bits == 64 or abs(lval) > 9007199254740991):
            return False
        return lval == rval
    # Large mantissas above double's exact integer range can trigger Zend's
    # overflow fallback. Preserve an unknown result in that case.
    mantissas = (left.lower().split(b"e", 1)[0], right.lower().split(b"e", 1)[0])
    if max(abs(lval), abs(rval)) > 9007199254740991 and any(len(x.replace(b"+", b"").replace(b"-", b"").replace(b".", b"")) > 15 for x in mantissas):
        return None
    return lval == rval


def response(output, expected_headers, bits):
    """Return raw body, header comparison, and whether header separator was found."""
    match = re.match(rb"^(.*?)\r?\n\r?\n(.*)", output, re.DOTALL)
    if match:
        raw_headers, body = match.group(1), match.group(2)
    else:
        raw_headers, body = b"", output
    def parse_headers(data):
        parsed = {}
        for line in re.split(rb"[\n\r]+", data):
            if b":" in line:
                name, value = line.split(b":", 1)
                parsed[name.strip(_TRIM)] = value.strip(_TRIM)
        return parsed
    expected = parse_headers(expected_headers)
    actual = parse_headers(raw_headers)
    for name, value in expected.items():
        if name not in actual:
            return body, False, bool(match)
        equal = _numeric_equal(value, actual[name], bits)
        if equal is None:
            return body, None, bool(match)
        if not equal:
            return body, False, bool(match)
    return body, True, bool(match)
