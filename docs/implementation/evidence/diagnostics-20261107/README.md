# PR5 diagnostics evidence (2026-11-07)

Base origin/main dcb97a0. Commands and results:

- `gofmt -l .`: no output. `CGO_ENABLED=0 go build ./...` for linux/amd64, linux/386, windows/amd64, darwin/arm64: ok. `go vet ./...`: ok.
- `CGO_ENABLED=0 go test -count=1 ./...`: all packages ok.
- `go test -race` per package: `race.log`, 12 packages ok.
- `python3 -m unittest discover -s tools -p 'test_*.py'`: `python.log`. The first run had one timeout in `test_plan_swapped_to_fifo_after_validation_is_rejected_without_hanging` (5 s limit) at load average about 50; the test passed alone on this branch in 1.25 s. `python-rerun.log` is the full rerun.
- `python3 tools/validate_docs.py`: no errors.
- `red-api.log`: the new public API and bytecode tests failed to build before the implementation (undefined `Diagnose`, `ConfigError.Code`).

## After the review fixes (63c6a67)

- `red-review1.log`: the new review tests failed before the fixes (renderer panic on a column past the end, locate showing rejected positions, nondeterministic Diagnose, missing reserved-word note).
- `test-review1.log`: `CGO_ENABLED=0 go test -count=1 ./...` exit 0.
- `race-review1.log`: per-package race run, all ok.
- `python-review1.log`: 553 tests OK when run alone. The earlier `python-rerun.log` had two signal-test errors while other suites ran at the same time; both passed when rerun alone, and this PR does not change `tools/`.
