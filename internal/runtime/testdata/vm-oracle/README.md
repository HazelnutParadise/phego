# W-17 VM oracle data

PHP 8.5.12 observations for the stage-1 interpreter tests (`vm_oracle_test.go`, `vm_snippets_test.go`), covering operations the operator reference `../php8512-ops-*.json` does not: `xor`, `(array)`, `++`/`--`, array key normalization, reads, writes, `isset` and `unset` on every container type, backtrace argument rendering, `CallValue` errors and `exit` forms. `tables.php` prints the tables; each `snippets/*.php` runs as a program and its display-mode stdout and exit code are recorded.

Regenerate with the pinned images already present locally (nothing is pulled; containers run without network): `python3 -I internal/runtime/testdata/vm-oracle/generate.py` (both widths; pass `64` or `32` for one). The 32-bit image runs under emulation on Apple Silicon.
