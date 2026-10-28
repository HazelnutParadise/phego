# Phego - The safest PHP in the world

PHP, reimagined in Go. Phego is a project for a security-first PHP compiler and execution engine that fits inside a Go application, serves existing PHP websites, and carries PHP’s entire history forward.

## One engine, from the first PHP to what comes next

Phego is designed around Go-native embedding, explicit PHP semantics, isolated request state, and verifiable execution boundaries. Its internal architecture is free to evolve without inheriting Zend’s implementation.

The product targets are:

- **Every PHP generation.** Compatibility reaches back to the first publicly released PHP Tools and PHP/FI, with precise release and platform profiles rather than approximate modern-PHP compatibility.
- **PHP inside Go.** Execute PHP through a Go function, pass values through explicit ownership boundaries, and expose only the host capabilities an application author authorizes.
- **Existing websites, familiar infrastructure.** Keep Apache or Nginx and connect through FastCGI. Serve independent requests with bounded goroutine concurrency and isolated execution state.
- **Security throughout execution.** Memory-safe core design, capability-controlled resources, data-origin tracking, bounded resource use, and consistent enforcement across execution backends.
- **Performance leadership, demonstrated.** Outperform every applicable existing PHP compiler and engine in reproducible, correctness-checked comparisons. No cherry-picked averages, disabled protections, or unsupported workloads counted as wins.

A pure-Go core and bytecode VM provide the baseline execution path. Optional LLVM native compilation and application bundling build on the same semantics and security contracts. Native execution does not eliminate the required runtime; the runtime can be packaged with the application.

## Proven on real applications

Acceptance is based on real WordPress and WooCommerce installations, Laravel framework tests and a real Laravel application such as BookStack, other PHP ecosystems, and authentic historical applications.

The acceptance plan checks installation, authentication, authorization, content editing, database changes, uploads, sessions, queues, scheduled tasks, upgrades, rollback, concurrency, and failure recovery. A page that renders is not enough. Application code must remain unchanged for an unmodified-application compatibility claim.

## Project status

**Specification stage.** This repository contains the architecture specification, implementation work packages, acceptance plans, and documentation validation tooling. It does not yet contain a runnable PHP engine.

The slogan is a playful riff on the classic 'PHP is the best language in the world' meme. Full compatibility, security leadership, and performance leadership remain acceptance goals, not completed certifications. Product tests and benchmarks remain unexecuted.

## Start with the specification

| Document | Purpose |
|---|---|
| [Documentation index](docs/README.md) | Navigate the complete design and evidence rules. |
| [Full specification, Traditional Chinese](docs/spec/specification.zh-TW.md) | The standalone 40-chapter specification and implementation plan. |
| [Compatibility contract](docs/compatibility/README.md) | Every historical release, explicit profiles, and evidence requirements. |
| [Benchmark contract](docs/benchmarks/README.md) | Complete competitor coverage and strict, reproducible comparisons. |
| [Real-application acceptance](docs/acceptance/README.md) | Real projects, data integrity, and release gates. |
| [Implementation issues and draft PRs](docs/implementation/README.md) | Complete work-package mapping, dependencies, and acceptance ownership. |
| [Agent instructions](AGENTS.md) | Implementation boundaries, traceability, and completion rules. |
| [Change log](CHANGELOG.md) | Specification changes and superseded decisions. |

The plan contains **100 requirements, 200 acceptance scenarios, 60 work packages, and 28 architecture decisions**. Machine-readable records are in [docs/data](docs/data/).

Validate documentation structure and traceability with Python 3.10 or later:

```sh
python3 tools/validate_docs.py
```

This command validates the documentation package. It does not run PHP, certify applications, or measure engine performance.

## License

Phego’s original code and project documentation are licensed under the [MIT License](LICENSE). Third-party software, tests, and application fixtures retain their own licenses; see [Third-party notices](THIRD_PARTY_NOTICES.md).
