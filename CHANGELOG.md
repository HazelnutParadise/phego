# Specification change log

## 1.1.2 · 2026-10-28

Replaced the active brand tagline with the founder's exact wording: **“The safest PHP in the world”**. Its lowercase styling and word order are intentional and retain the original PHP-language meme reference. This is a product slogan, not a security certification. The implementation plan, requirements, and acceptance goals are unchanged.

## 1.1.1 · 2026-10-28

Slogan-only clarification. The English line is now **“The World's Safest PHP.”**, echoing the Chinese joke that “PHP is the best language in the world.” This is brand humor, not a verified security superiority claim. Requirements, tests, and work packages are unchanged.

## 1.1 · 2026-10-28

This is a specification revision, not a software release.

### Confirmed scope changes

- Expanded mandatory compatibility from a modern PHP starting profile to the entire publicly released PHP history, including the first PHP Tools and PHP/FI. Exact releases, historical language generations, and 32/64-bit guest semantics require explicit evidence.
- Made outperforming every applicable existing PHP compiler and execution engine a product acceptance goal, with complete competitor discovery and per-workload evidence.
- Replaced minimal demonstration-site acceptance with real projects, official tests, browser journeys, data integrity, fault injection, migration, and rollback. WordPress/WooCommerce and Laravel/BookStack are explicit required project lines.
- Set the target repository to `HazelnutParadise/phego`, the original-work license to MIT, the README language to English, and the slogan to “The World’s Most Secure PHP.”

### Planning changes

- Added chapters 36-40 and updated the earlier chapters in place so they do not retain the superseded scope.
- Expanded from 79 to 100 requirements, 158 to 200 planned acceptance scenarios, 48 to 60 work packages, and 22 to 28 decisions.
- Added historical compatibility, competitor, and real-application planning catalogs. These are not fabricated source locks or completed results.
- Added executable documentation validation and repository-publication instructions.

### Superseded decision

ADR-16's restricted modern-PHP starting scope is superseded by ADR-23. A staged implementation may produce explicitly scoped previews, but it cannot redefine the complete product target.

### Evidence status

No Phego engine, PHP acceptance tests, real-application certification, or performance benchmark has been executed by this documentation revision. Publication status is recorded separately in the delivery report, not inferred from a prepared repository URL.

## 1.0 · 2026-10-28

Initial standalone architecture specification and implementation plan with 35 chapters, 79 requirements, 158 acceptance scenarios, 48 work packages, and 22 decisions.
