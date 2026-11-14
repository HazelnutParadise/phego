# Phego implementation contract

## Read before changing the project

Read `engineering/spec/specification.zh-TW.md`, especially chapters 01-11, 22-28, and 36-40. Load the requirement, acceptance-test, and work-package JSON records under `engineering/data/`. This repository is at the specification stage; do not mistake planned APIs for implemented APIs.

Before selecting a work package, read `engineering/implementation/README.md` and `engineering/implementation/github-index.json` for its issue, draft PR, integration dependencies, and acceptance owner. Read the package plan on its draft branch before implementing it.

## Fixed product direction

1. Keep the core Go-native and embeddable through a Go function. The default core must build without cgo or LLVM. Optional native builds must disclose their dependencies.
2. Design for Go’s properties rather than translating Zend internals mechanically. Preserve PHP-visible semantics, explicit ownership, and safety invariants.
3. Support PHP’s entire publicly released history starting with the first PHP Tools. Missing archives and unverified releases remain open blockers; they are not grounds to redefine the starting version.
4. Use bounded goroutine concurrency for independent requests. Keep mutable execution state request-local; goroutines are not hard security sandboxes.
5. Preserve FastCGI integration with existing Apache and Nginx installations. No production fallback to the Zend engine.
6. Retain the performance goal of beating every applicable existing PHP compiler and engine. A competitor that cannot run a workload is not a performance win. An average win cannot erase losing required cells.
7. Validate actual applications, including WordPress/WooCommerce, Laravel and BookStack, other ecosystems, and historical applications. Do not modify application PHP code to hide incompatibilities.
8. Keep the English README and the slogan “The safest PHP in the world” Distinguish branding and product targets from measured claims.
9. Keep the MIT license for original project work and preserve all third-party licenses and attribution.

## Implementation and evidence

Work from the dependency graph, but start historical-source discovery and competitor discovery early. Record the exact requirement and test IDs touched by each change. Add executable tests and reproducible evidence before claiming a work package has passed acceptance.

The initial state is `planned` for requirements, `not_run` for acceptance scenarios, and `not_started` for implementation work packages. Documentation validation is not implementation acceptance. Do not turn failures into skips, weaken expected behavior, fill in invented checksums, or report results that were not executed.

Use a precise release/platform profile throughout parsing, compilation, runtime behavior, caches, modules, and native artifacts. Do not share request static variables, environments, sessions, mutable objects, or failure state through a compiled-code cache.

Keep security checks and cancellation semantics intact under optimization. Native code must not bypass capability checks or access arbitrary Go-managed memory. Treat host callbacks as trusted host code, not as automatically sandboxed operations.

Historical reference engines, application fixtures, and potentially unsafe tests run in disposable restricted environments without production data or secrets. Use test doubles or authorized sandboxes for payments, email, and external effects.

Run `python3 tools/validate_docs.py` after changing planning records. Product test commands must be added when a runnable engine exists; do not invent passing CI for absent implementation.

## Diagnostics

Phego's own diagnostics follow the rustc style: they are explicit, located and actionable. This covers compiler and verifier errors, configuration and policy errors, CLI and tool output, and messages written for embedding hosts. Each diagnostic carries a stable error code, a one-line message saying what is wrong, the location as `file:line:column`, the relevant source with markers under the exact span, and `note:` and `help:` lines that explain the cause and the fix. For example:

```
error[PHG0123]: mount path is not absolute
  --> phego.toml:12:9
   |
12 |   path = "app"
   |          ^^^^^ expected a path starting with "/"
   |
   = note: mounts are matched against absolute PHP paths
   = help: write "/app"
```

Go error values keep the code, span and notes as structured fields so hosts can render or inspect them; `Error()` stays a single line. Output that PHP defines, such as warnings, fatal errors, exception messages and their line numbers, stays byte-identical to the selected PHP profile. A Rust-style view of PHP diagnostics may only be an explicit opt-in that never changes what PHP code or its users observe by default.

## Review

Every review must look for approaches that cost the same or less and give a better result, and report each one it finds. Take every such benefit where PHP compatibility does not constrain the internals; it is not a trade-off to weigh. Judge approaches with added cost by the benefit they actually bring. Conventional code is not acceptable just because it works. This never overrides PHP-visible semantics or safety checks.

## Decision boundaries

You may choose private types, internal layout, algorithms, test tooling, and evidence-backed optimizations within the specification. Document meaningful design changes.

Obtain approval before reducing compatibility, removing safety checks, requiring cgo/LLVM for the default core, changing the Go embedding contract, adding production Zend fallback, changing project licensing, modifying production infrastructure, or introducing paid dependencies. Repository creation and publication of these specification files to `HazelnutParadise/phego` are authorized; unrelated repositories, production deployment, and software releases are not authorized by that instruction.

Read the current remote branch before edits when a remote exists. Preserve existing changes, use normal commits, and do not force-push. A local commit is not proof of a successful remote push.

## Handoff

Report changed requirements, implementation locations, actual commands and results, remaining gaps, compatibility and safety implications, and the next dependency-satisfied work package. Keep unknown and blocked states visible. Do not rely on the chat transcript for decisions missing from the repository.
