# PHP vulnerability regression corpus

Each program in `cases/` replays the trigger of one published PHP vulnerability. Patched PHP 8.5.12 runs it in the pinned images and Phego must produce the same stdout, displayed errors, logged errors and exit code. `TestPHPCVERegressions` in `security_php_cve_test.go` runs every case in all three error modes on every profile (W-41; REQ-067, T-133, T-134).

The list of vulnerabilities, and which ones have a case, is [the catalog](../../../engineering/compatibility/php-cve-catalog.json). A case passing shows that Phego handles that input the way patched PHP does. It does not show that Phego is free of the whole class of bug.

## What a case may check

Most PHP vulnerabilities are C memory errors: a buffer overflow, a use-after-free, an integer overflow that corrupts the heap. Go's bounds checks and garbage collector rule out the corruption itself, so a case for such a bug checks what is left to get wrong: the same input must end in PHP's fixed output, not in a Go panic, a hang, a stack overflow or a different value. A logic bug, such as a declare value PHP rejects or an `unset()` that removes the wrong key, is checked by its observable result in the same way.

A case uses only what the engine already runs (the syntax listed in `tests/vm/w17/README.md`). A vulnerability in a function, extension or SAPI that Phego does not have yet stays `pending` in the catalog with the work package that will add it; its case is written with that package, not skipped here.

When upstream fixed a vulnerability after the pinned release, PHP 8.5.12 still has the bug, so its output is not the expectation. Such a case waits until the reference images move to a fixed release, and the catalog entry keeps a note of the behaviour the fix requires.

## Adding a case

1. Name the file `cve_<year>_<number>_<what it does>.php` (or `ghsa_...` for an advisory without a CVE).
2. Start with `<?php`, optional `// ini: name=value` lines, then one `// CVE: CVE-YYYY-NNNN` line for every vulnerability the case covers, then a short comment saying what the original bug did.
3. List the case under each of those identifiers in `engineering/compatibility/php-cve-triage.json` (`"tests": [...]`). A case that covers only part of a vulnerability, for example the opcode path but not the functions named in the same advisory, says so in the entry's `note` and sets `"coverage": "partial"`.
4. Regenerate the references and the catalog, then run the checks:

```sh
python3 tests/security/php-cve/reference.py --width 64 --write
python3 tests/security/php-cve/reference.py --width 64 --arch amd64 --write
python3 tests/security/php-cve/reference.py --width 32 --write
python3 tools/php_cve_catalog.py build --cache <directory written by fetch>
python3 tools/php_cve_catalog.py check
go test -run '^TestPHPCVERegressions$' .
```

`reference.py` stages `cases/` next to the W-17 runner (`tests/vm/w17/reference.php`) and records `reference-64.json`, `reference-64-amd64.json` and `reference-32.json` in the same format as the W-17 references. Without `--write` it regenerates the reference and fails on any drift. The amd64 and 386 images run under emulation on Apple Silicon and take a few minutes.

The catalog check fails when a case file names an identifier the triage does not list it under, or the other way round, so a case cannot be added or removed without the catalog and the README numbers following.
