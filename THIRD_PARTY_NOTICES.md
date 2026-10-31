# Third-party sources and licenses

Phego's original code and project documentation are covered by the root MIT license. That license does not relicense third-party components, PHP sources, upstream tests, application fixtures, datasets, or documentation.

This specification package does not vendor PHP, WordPress, WooCommerce, Laravel, BookStack, LLVM, or their runtime/application sources. References identify behavior, documentation, and future validation targets. Upstream projects retain their names, copyrights, and licenses.

Before copying or deriving any third-party code, tests, or fixtures, record its upstream URL, exact version/commit, retrieved artifact digest, applicable license, modifications, and attribution requirements. Maintain the notices with the resulting distribution. Prefer fetching pinned test targets into an isolated test workspace rather than implying that third-party application code is part of the project's MIT grant.

The complete reference register is [docs/data/phego-sources.json](docs/data/phego-sources.json). Sources carried forward from the original specification are distinguished from sources checked for this revision. A [partial source lock](compat.lock.json) and [license-document inventory](docs/implementation/evidence/W-02-license-probe.md) are available. Complete source locking and license review are still required before implementation imports or redistributes third-party artifacts.

The MIT license text is available from the [Open Source Initiative](https://opensource.org/license/mit).

## Arm optimized-routines

`internal/runtime/pow.go` is a Go translation of `math/pow.c`, and `internal/runtime/pow_data.go` holds constants and tables taken from `math/pow_log_data.c` and `math/exp_data.c`, of [ARM-software/optimized-routines](https://github.com/ARM-software/optimized-routines) at commit `2ee4b0d45680a9880412c1d186224b08652c35f6`. glibc uses this pow algorithm; the translation reproduces glibc 2.41 on arm64 so PHP's `**` gives the same results. Copyright (c) 2018-2026, Arm Limited (`math/pow.c`) and 2018-2023 (`math/exp_data.c`, `math/pow_log_data.c`); the license file names 1999-2022. Used under the MIT License, one of the offered licenses `MIT OR Apache-2.0 WITH LLVM-exception`; the license text is in [third_party/optimized-routines/LICENSE](third_party/optimized-routines/LICENSE).

Retrieved file SHA-256: `math/pow.c` `4497df6d64e0043c04df2cb308ae2918922b0f53e10f133006fba4e9eb5e8fb7`, `math/pow_log_data.c` `d28818bd95355e6854766a3818a6ec0866f7afcef592fe1e836d05d3f465392e`, `math/exp_data.c` `a5e84677e7693c7d22195f86faac5fdc6f92bff7ea2e54c0e7888203a78ffc24`, `LICENSE` `650afbf29f214451e02241adc42534e82c9d6ae2b38e2444b92b5a1ffcaf9346`.

Modifications: translated to Go; only the 128-entry tables and fused multiply-add path are kept; the multiply-add contraction of the arm64 glibc build, checked against its disassembly, is written as explicit `math.FMA`; errno and floating-point exception signaling are omitted because PHP does not observe them.
