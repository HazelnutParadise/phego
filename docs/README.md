# Documentation

The design baseline is specification **1.1.2**, dated **2026-10-28**. The detailed specification is in Traditional Chinese; the repository overview and agent instructions are in English.

## Primary documents

- [Full specification and implementation plan](spec/specification.zh-TW.md): 40 chapters, including all historical PHP releases, performance leadership, real-application acceptance, and repository rules.
- [Compatibility contract](compatibility/README.md): target generations and exact-release evidence.
- [Benchmark contract](benchmarks/README.md): competitor discovery, fairness, statistics, and completion gates.
- [Application acceptance](acceptance/README.md): genuine applications, source locks, workflows, and data invariants.
- [Decision register](decisions/README.md): all 28 decisions and their status.

## Machine-readable planning records

| File | Content | Initial state |
|---|---|---|
| [Requirements](data/phego-requirements.json) | 100 stable requirement IDs | planned |
| [Acceptance scenarios](data/phego-acceptance-tests.json) | 200 positive/negative scenarios | not_run |
| [Work packages](data/phego-work-packages.json) | 60 packages with dependencies | not_started |
| [Historical policy](data/php-history-policy.json) | Required generations, inventory rules, and missing-evidence handling | planning_catalog |
| [Competitor catalog](data/competitor-catalog.json) | Initial discovery targets; not an exhaustive completed inventory | planning_catalog |
| [Application catalog](data/application-catalog.json) | Real application targets and required workflows | planning_catalog |
| [Sources](data/phego-sources.json) | Referenced primary documentation and source provenance | source_register |
| [Document validation](data/document-validation.json) | Structural, link, state, and traceability checks | documentation_only |

Run `python3 tools/validate_docs.py` from the repository root. The command prints and records document-check results, not engine test results. Do not replace unexecuted product tests with this report.

## Where to begin implementation

Read chapters 01-11, 22-28, and 36-40, then [AGENTS.md](../AGENTS.md). Establish exact upstream source locks, language profiles, isolated oracles, and competitor coverage before claiming compatibility or performance. Work-package IDs describe deliverables and dependencies, not completed code.
