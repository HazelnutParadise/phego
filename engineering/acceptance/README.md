# Real-application acceptance

Acceptance must run real applications, not only self-written demos, language tests, or a homepage. The initial required lines include WordPress/WooCommerce, Laravel framework tests plus BookStack, Drupal, a real Symfony application, MediaWiki, phpMyAdmin, phpBB, and authentic historical PHP applications.

See the [application catalog](../data/application-catalog.json). It records targets and workflows, not pinned source artifacts or completed tests. Each real test run must first lock actual upstream versions, source digests, dependency locks, PHP/platform profiles, modules, databases, configuration, and fixtures.

## Required behavior

Check installation, login/logout, password reset, permissions, content and form workflows, database writes, uploads, sessions, cache behavior, APIs, CLI, queues, scheduled work, upgrades, and rollback where applicable. Test the actual Apache/Nginx FastCGI path. Check output and database/filesystem/external-event effects, not just HTTP status.

Application code remains unchanged for unmodified-application certification. The application under test must really execute on Phego, including PHP test-framework and background-worker entrypoints. An external oracle or orchestration tool may use official PHP, but it must not execute the functionality attributed to Phego.

Use matching historical versions: a modern framework is not a meaningful PHP 1 fixture. When an authentic old application or reference engine is unavailable, preserve the evidence gap instead of substituting a fabricated success.

## Reliability and integrity

Run concurrent, mixed-site, mixed-version tests; inject cancellation, failed initialization, database outages, disk exhaustion, upload interruption, reloads, and worker failure. At minimum, include a 24-hour run and one million mixed requests, then inspect resource trends and data invariants. Longer or more extensive tests may be required for a workload.

Use disposable test databases and authorized service sandboxes. Never run live payments or duplicate side effects against production systems. A compiler fallback must not repeat an already committed action.

## Completion

All required cases within the declared scope must pass with no unresolved failures, crashes, timeouts, corruption, authorization failures, unknown support, or runner gaps. Approved environmental inapplicability and security differences remain explicit; they are not silently reported as successful equivalent behavior.

Passing finite tests does not prove that no possible bug exists. It establishes the exact tested scope and evidence, which must be maintained as the engine changes. Full requirements are in [chapter 38](../spec/specification.zh-TW.md#chapter-38).
