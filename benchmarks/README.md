# Benchmark plan

`plan.json` is the frozen W-54 comparison matrix: every inventoried competitor × every workload × platform × mode. All 504 cells are `pending`; no comparison has run and nothing here is a performance result. Cells may move to `comparable` or `unsupported` only with evidence, and none may be removed after measurement. Validate and freeze it with:

```sh
python3 tools/competitors/registry.py validate --root "$PWD" --plan "$PWD/benchmarks/plan.json"
python3 tools/competitors/registry.py freeze --root "$PWD" --plan "$PWD/benchmarks/plan.json" --output comparison.phego
```

`workloads/*.php` are engine-level CLI workloads written in syntax accepted by PHP 5 and later so older engines can run them. Each prints a checksum; `*.expected` is the PHP 8.5.12 output and is the correctness oracle. A run whose output differs is not counted as throughput. All intermediate values stay below 2^31, so the output should not depend on integer width; that is unverified on a native 64-bit reference.

Known coverage gaps are tracked in `coverage-gaps.json`.
