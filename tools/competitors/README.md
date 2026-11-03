# Competitor plan archive

`registry.py` validates a supplied plan, freezes its referenced files into one archive and checks an archive without extracting it. This is a structural helper for REQ-088 and REQ-089, not evidence that the competitor inventory, required workload matrix or product acceptance is complete. It does not run an engine or benchmark.

```sh
python3 tools/competitors/registry.py validate --root INPUT_ROOT --plan PLAN.json
python3 tools/competitors/registry.py freeze --root INPUT_ROOT --plan PLAN.json --output comparison.phego
python3 tools/competitors/registry.py verify --bundle comparison.phego
```

The plan contains `version: 1`, nonempty unique string lists `competitors`, `workloads`, `platforms`, and `modes`, plus `cells` and `files`. Each cell has `competitor`, `workload`, `platform`, `mode`, `status`, and `evidence`. Cells must cover the Cartesian product exactly once. `status` is `pending`, `comparable`, or `unsupported`; `unsupported` requires evidence. Each `files` entry contains a relative `path` and its lowercase `sha256`. `pending` and `unsupported` are counted as blockers. The structure does not establish the truth of a status, the completeness of dimensions, equivalence, or measurement results.

The archive is created exclusively and existing destinations are not overwritten. A failed write may leave an incomplete file. Verification checks the bytes it reads; an embedded hash is not an independent signature and cannot establish provenance or prevent a same-user process from altering the file or replacing its path. Input and output path components must not be symlinks; supported systems must provide no-follow directory file descriptors. The tool rejects oversize files and malformed archives.

Run the tool's tests with `python3 -m unittest discover -s tools -p 'test_*.py'`. Tests use synthetic inputs and do not constitute product acceptance.
