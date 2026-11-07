# PR6 display_errors evidence (2026-11-07)

Base origin/main 78c4e01.

- References: `python3 tools/lifecycle_reference.py --width 64 --arch arm64 --write`, `--width 32 --arch 386 --write`, `--width 64 --arch amd64 --write` with the pinned images; each reports 60 cases. Each fixture now also runs with `display_errors=1 log_errors=1 html_errors=0`.
- `display-differs.txt`: in 23 of 60 cases per profile the display_errors=1 stdout differs from the stdout the runtime produced before this change (diagnostics kept apart from write()).
- `go test -run TestExecutionOracleReplay ./internal/runtime`: both modes pass for the 64, 32 and amd64 profiles (stdout byte-identical, exit codes, log level order and position among events).
- `test.log`: `CGO_ENABLED=0 go test -count=1 ./...`.
- gofmt, `go vet ./...`, builds for linux/amd64, linux/386, windows/amd64, darwin/arm64: ok. `python3 -m unittest discover -s tools -p test_lifecycle_reference.py`: 8 OK.
