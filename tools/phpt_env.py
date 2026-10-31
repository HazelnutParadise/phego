"""Parse the PHP 8.5.12 run-tests.php PHPT ENV section."""
import re


MAX_ENV_BYTES = 64 * 1024
MAX_ENV_LINES = 256
_NAME = re.compile(rb"[A-Za-z_][A-Za-z0-9_]*\Z")
_PHP_TRIM = b" \t\n\r\0\x0b"
_RESERVED = {
    b"REQUEST_METHOD", b"QUERY_STRING", b"PATH_TRANSLATED", b"SCRIPT_FILENAME",
    b"TEST_PHP_EXTRA_ARGS", b"REDIRECT_STATUS", b"HTTP_COOKIE", b"CONTENT_TYPE",
    b"CONTENT_LENGTH",
}


def parse_env(data: bytes, release: str) -> tuple[bytes, ...]:
    """Return canonical NAME=value byte arguments for the supported release."""
    if not isinstance(data, bytes):
        raise TypeError("ENV input must be bytes")
    if release != "8.5.12":
        raise ValueError("unsupported PHP release")
    if len(data) > MAX_ENV_BYTES:
        raise ValueError("ENV input exceeds 64 KiB")
    if b"\0" in data:
        raise ValueError("ENV input contains a NUL byte")
    lines = data.split(b"\n")
    if len(lines) > MAX_ENV_LINES:
        raise ValueError("ENV input exceeds 256 lines")
    values = {}
    for line in lines:
        line = line.strip(_PHP_TRIM)
        if b"=" not in line:
            continue
        name, value = line.split(b"=", 1)
        if not name:
            continue
        if not _NAME.fullmatch(name):
            raise ValueError("invalid ENV variable name")
        if name in _RESERVED:
            raise ValueError("reserved ENV variable name: " + name.decode("ascii"))
        values[name] = value.replace(b"{PWD}", b"/work")
    return tuple(name + b"=" + value for name, value in values.items())
