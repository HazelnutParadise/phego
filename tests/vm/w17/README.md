# W-17 differential VM fixtures

`cases/*.php` are small PHP programs for the first Phego VM stage. Each one runs in the pinned PHP 8.5.12 images and in Phego, and stdout, displayed errors, logged errors and exit codes must match byte for byte. The references are oracle data, never product acceptance.

Fixtures use only the syntax the first four VM stages support: echo, print, literals, simple interpolation, variables, operators, casts, `isset`/`unset`, array literals, array element reads and writes (compound assignment, increment and decrement, nested and appending writes, nested `unset`), `foreach` by value and by reference, assignment by reference, destructuring (`[...] =`, `list()`, keyed, nested, by reference, in `foreach`), string offset reads, control flow, `exit`/`die`, top-level user functions with fixed positional parameters (by value or by reference) and constant-expression default values, and top-level `return`. The stage-2 groups are named `array_elem_*`, `foreach_*`, `param_default_*` and `string_offset_*`; the stage-3 groups, for references, are `ref_*`, `foreach_ref_*` and `param_ref_*`; the stage-4 groups, for destructuring, are `list_*` and `foreach_list_*`; their uncaught-error and compile-error variants start with `fatal_`. They call no builtin functions, and they must not depend on time, memory, hash order or host paths. A case that needs anything else waits for a later stage.

`reference.php` runs inside the container. For each case, in sorted order, it starts `php -n` with stdin closed, working directory `/w` and the script at `/w/cases/<name>.php`, in three modes that all set `html_errors=0` and `error_reporting=-1`:

- `display`: `display_errors=1`, `log_errors=0`
- `log`: `display_errors=0`, `log_errors=1`, `error_log=` (the CLI writes bare `PHP Warning:  ...` lines to stderr, without a timestamp)
- `both`: `display_errors=1`, `log_errors=1`, `error_log=`

Cases named `return_*` run a fourth time through `include`, and the top-level return value is recorded as JSON text in `return_json` (`JSON_PRESERVE_ZERO_FRACTION`). Every other case has `return_json: null`.

Regenerate a profile with `python3 tests/vm/w17/reference.py --width 64 --write` (arm64, `reference-64.json`), `--width 32 --write` (386, `reference-32.json`) or `--width 64 --arch amd64 --write` (`reference-64-amd64.json`). Without `--write` the script regenerates and fails on any drift from the committed file. 386 and amd64 run under emulation on Apple Silicon and take several minutes.

Each reference file holds `php_version`, `int_size`, `os`, `machine`, `image`, `platform`, the scrubbed `command`, `inputs_sha256` and `cases`. Each case holds `case`, `source_sha256`, `display`, `log` and `both` (each with `stdout_b64`, `stderr_b64` and `exit_code`) and `return_json`. `machine` is the kernel's name (a 386 container reports `i686` under emulation and `x86_64` on an x86_64 kernel), so drift comparison skips it.
