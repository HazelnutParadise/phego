"""Compare PHPT expectations using the pinned PHP PCRE implementation."""

import math
import re

from phpt_session import run_case


_MAX_INPUT = 1024 * 1024
_RELEASE = "8.5.12"
_PLATFORMS = frozenset(("linux/386", "linux/amd64", "linux/arm64"))
_MARKER_TRUE = b"PHEGO_PHPT_PCRE_MATCH"
_MARKER_FALSE = b"PHEGO_PHPT_PCRE_MISMATCH"
_MARKER_ERROR = b"PHEGO_PHPT_PCRE_ERROR"

# This source is constant. The only caller data enters PHP as the two files in
# the fixture archive prepared by run_case, never through argv or PHP source.
_PHP_MATCHER = b'''<?php
$expected = file_get_contents('/work/expected.bin');
$actual = file_get_contents('/work/actual.bin');
if ($expected === false || $actual === false) { exit(91); }
$formatted = __FORMATTED__;
if ($formatted) {
    $pattern = '';
    $cursor = 0;
    $length = strlen($expected);
    while ($cursor < $length) {
        $open = strpos($expected, '%r', $cursor);
        if ($open === false) {
            $pattern .= preg_quote(substr($expected, $cursor), '/');
            break;
        }
        $pattern .= preg_quote(substr($expected, $cursor, $open - $cursor), '/');
        $close = strpos($expected, '%r', $open + 2);
        if ($close === false) {
            $pattern .= preg_quote(substr($expected, $open), '/');
            break;
        }
        $pattern .= '(' . substr($expected, $open + 2, $close - $open - 2) . ')';
        $cursor = $close + 2;
    }
    if ($length === 0) { $pattern = ''; }
    $pattern = strtr($pattern, array(
        '%e' => preg_quote('/', '/'),
        '%s' => "[^\\r\\n]+",
        '%S' => "[^\\r\\n]*",
        '%a' => '.+?',
        '%A' => '.*?',
        '%w' => '\\s*',
        '%i' => '[+-]?\\d+',
        '%d' => '\\d+',
        '%x' => '[0-9a-fA-F]+',
        '%f' => '[+-]?(?:\\d+|(?=\\.\\d))(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?',
        '%c' => '.',
        '%0' => '\\x00',
    ));
} else {
    $pattern = $expected;
}
$result = @preg_match('/^' . $pattern . '$/s', $actual);
if ($result === false) {
    echo 'PHEGO_PHPT_PCRE_ERROR';
} elseif ($result === 1) {
    echo 'PHEGO_PHPT_PCRE_MATCH';
} else {
    echo 'PHEGO_PHPT_PCRE_MISMATCH';
}
?>
'''


def _gap(message="PHP PCRE reference comparison is unavailable"):
    return None, "PHPT PCRE: " + message


def match_reference(expected, actual, formatted, image, executable, platform, release, timeout):
    """Return (matched, diagnostic); matched is True, False, or None on a gap."""
    if type(expected) is not bytes or type(actual) is not bytes or type(formatted) is not bool:
        return _gap("invalid PHP PCRE input types")
    if len(expected) > _MAX_INPUT or len(actual) > _MAX_INPUT:
        return _gap("PHP PCRE input exceeds 1 MiB")
    if type(release) is not str or release != _RELEASE or type(platform) is not str or platform not in _PLATFORMS:
        return _gap("unsupported PHP PCRE reference profile")
    if (type(image) is not str or not re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", image)
            or type(executable) is not str or not executable.startswith("/") or "\x00" in executable):
        return _gap("invalid PHP PCRE reference configuration")
    if (type(timeout) not in (int, float)
            or (type(timeout) is float and not math.isfinite(timeout))
            or timeout <= 0 or timeout > 120):
        return _gap("invalid PHP PCRE reference timeout")

    source = _PHP_MATCHER.replace(b"__FORMATTED__", b"true" if formatted else b"false")
    try:
        observation = run_case(
            image, executable, platform, release, source, b"",
            {"expected.bin": expected, "actual.bin": actual}, timeout, 64,
        )
    except Exception:
        return _gap()

    if (getattr(observation, "status", None) != "completed"
            or getattr(observation, "cleanup_confirmed", None) is not True):
        return _gap()
    profile = getattr(observation, "profile", None)
    skipif = getattr(observation, "skipif", None)
    program = getattr(observation, "program", None)
    if not _phase_is(profile, "completed", release.encode("ascii") + b"\n" +
                     (b"4" if platform == "linux/386" else b"8"), 0):
        return _gap("PHP PCRE reference profile did not verify")
    if not _phase_is(skipif, "completed", b"", 0):
        return _gap("PHP PCRE reference SKIPIF did not complete cleanly")
    if not _phase_is(program, "completed", getattr(program, "output", None), 0):
        return _gap("PHP PCRE reference program did not complete cleanly")

    output = getattr(program, "output", None)
    if output == _MARKER_TRUE:
        return True, "PHPT PCRE: expectation matched"
    if output == _MARKER_FALSE:
        return False, "PHPT PCRE: expectation did not match"
    return _gap("PHP PCRE rejected the expectation or returned an invalid marker")


def _phase_is(phase, status, output, exit_code):
    return (phase is not None
            and getattr(phase, "status", None) == status
            and getattr(phase, "output", None) == output
            and getattr(phase, "exit_code", None) == exit_code)
