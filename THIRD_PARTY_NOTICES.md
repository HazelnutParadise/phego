# Third-party sources and licenses

Phego's original code and project documentation are covered by the root MIT license. That license does not relicense third-party components, PHP sources, upstream tests, application fixtures, datasets, or documentation.

This specification package does not vendor PHP, WordPress, WooCommerce, Laravel, BookStack, LLVM, or their runtime/application sources. References identify behavior, documentation, and future validation targets. Upstream projects retain their names, copyrights, and licenses.

Before copying or deriving any third-party code, tests, or fixtures, record its upstream URL, exact version/commit, retrieved artifact digest, applicable license, modifications, and attribution requirements. Maintain the notices with the resulting distribution. Prefer fetching pinned test targets into an isolated test workspace rather than implying that third-party application code is part of the project's MIT grant.

The complete reference register is [docs/data/phego-sources.json](docs/data/phego-sources.json). Sources carried forward from the original specification are distinguished from sources checked for this revision. A [partial source lock](compat.lock.json) and [license-document inventory](docs/implementation/evidence/W-02-license-probe.md) are available. Complete source locking and license review are still required before implementation imports or redistributes third-party artifacts.

The MIT license text is available from the [Open Source Initiative](https://opensource.org/license/mit).
