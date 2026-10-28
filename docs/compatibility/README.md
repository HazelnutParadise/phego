# Complete historical PHP compatibility

The target includes every publicly released PHP generation from the first PHP Tools onward. It is not limited to PHP 5, 7, or 8. Exact release identities and patch-level behavior must be preserved, with separate entries for public prereleases and historical experimental snapshots.

PHP Tools, FI, Construction Kit, and PHP/FI require their own historical syntax and tool behavior. A modern parser with a version number switch is not sufficient. The official history and archives are starting evidence, not a completed release inventory. In particular, an available PHP 1.08 archive does not establish that the first public release has been recovered.

Use immutable language profiles that specify exact release, language generation, guest integer/platform semantics, modules, configuration behavior, security differences, and data versions. Keep those profiles in compilation, runtime, cache, and native artifact identity.

Known engine memory corruption and permission escapes are not compatibility features. Deliberate historical language behavior and security-policy restrictions must be distinguished. A policy denial that prevents a normal historical workflow must be reported, not counted as transparent compatibility.

The [history policy](../data/php-history-policy.json) is a planning catalog, not a list of certified releases. Build the exact source inventory and isolated reference environments under W-49. Never fabricate checksums, substitute an adjacent version, or remove an unavailable historical release from the goal.

Required evidence includes source identity, license, build environment, matching oracle, semantic tests, applicable real applications, and documented security differences. Unknowns and unavailable evidence remain completion blockers.

The authoritative behavior contract is in [specification chapters 36 and 40](../spec/specification.zh-TW.md#chapter-36). Background sources are the [official history](https://www.php.net/manual/en/history.php.php), [PHP 1 archive](https://museum.php.net/php1/), and [PHP/FI 2 manual](https://www.php.net/manual/phpfi2.php).
