#!/usr/bin/env python3
"""Validate Phego's planning documents, never claim engine-test coverage.

Run from any directory with Python 3.10+. Only the validation report is written.
The check is deterministic for a given set of input files.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit


def validate(root: Path) -> dict:
    errors: list[str] = []
    checks: list[dict[str, str]] = []
    data = root / "docs/data"
    document = root / "docs/spec/specification.zh-TW.md"

    def check(name: str, condition: bool, detail: str = "") -> None:
        checks.append({"check": name, "status": "pass" if condition else "fail", "detail": detail})
        if not condition:
            errors.append(f"{name}: {detail}")

    def load(name: str) -> object:
        try:
            return json.loads((data / name).read_text(encoding="utf-8"))
        except (OSError, ValueError) as exc:
            errors.append(f"{name}: {exc}")
            return {}

    def records(name: str, prefix: str, count: int, status: str) -> dict[str, dict]:
        obj = load(name)
        if not isinstance(obj, dict):
            check(name + " object", False)
            return {}
        items = obj.get("items", [])
        if not isinstance(items, list) or any(not isinstance(x, dict) for x in items):
            check(name + " records", False)
            return {}
        ids = [x.get("id") for x in items]
        width = 3 if prefix in {"REQ", "T"} else 2
        expected = {f"{prefix}-{i:0{width}d}" for i in range(1, count + 1)}
        check(name + " version", obj.get("document_version") == "1.1.2")
        check(name + " complete IDs", len(ids) == count and set(ids) == expected)
        check(name + " unique IDs", len(ids) == len(set(ids)))
        check(name + " unexecuted planning state", all(x.get("status") == status for x in items))
        return {x["id"]: x for x in items if isinstance(x.get("id"), str)}

    reqs = records("phego-requirements.json", "REQ", 100, "planned")
    tests = records("phego-acceptance-tests.json", "T", 200, "not_run")
    works = records("phego-work-packages.json", "W", 60, "not_started")

    relation_errors: list[str] = []
    for rid, req in reqs.items():
        tids, wids = req.get("tests", []), req.get("work_packages", [])
        if len(tids) != 2 or len(set(tids)) != 2 or not wids:
            relation_errors.append(rid + " missing tests/work packages")
        for tid in tids:
            if tid not in tests or tests[tid].get("requirement") != rid:
                relation_errors.append(rid + " -> " + str(tid))
        for wid in wids:
            if wid not in works or rid not in works[wid].get("requirements", []):
                relation_errors.append(rid + " -> " + str(wid))
    for tid, test in tests.items():
        rid = test.get("requirement")
        if rid not in reqs or tid not in reqs[rid].get("tests", []):
            relation_errors.append(tid + " has an invalid requirement")
        for field in ("preconditions", "procedure", "expected", "evidence"):
            if not test.get(field):
                relation_errors.append(tid + " lacks " + field)
    for wid, work in works.items():
        rids = work.get("requirements", [])
        expected_tests: set[str] = set()
        for rid in rids:
            if rid not in reqs or wid not in reqs[rid].get("work_packages", []):
                relation_errors.append(wid + " -> " + str(rid))
            else:
                expected_tests.update(reqs[rid]["tests"])
        if set(work.get("tests", [])) != expected_tests:
            relation_errors.append(wid + " has inconsistent test traceability")
        for dep in work.get("dependencies", []):
            if dep not in works:
                relation_errors.append(wid + " depends on unknown " + str(dep))
        for field in ("implementation_steps", "deliverables", "exit_gate"):
            if not work.get(field):
                relation_errors.append(wid + " lacks " + field)
    check("bidirectional requirement/test/work traceability", not relation_errors,
          "; ".join(relation_errors))

    colors: dict[str, int] = {}
    order: list[str] = []
    cycle_errors: list[str] = []
    def visit(wid: str, stack: list[str]) -> None:
        if colors.get(wid) == 1:
            cycle_errors.append(" -> ".join(stack + [wid]))
            return
        if colors.get(wid) == 2 or wid not in works:
            return
        colors[wid] = 1
        for dep in works[wid].get("dependencies", []):
            visit(dep, stack + [wid])
        colors[wid] = 2
        order.append(wid)
    for wid in works:
        visit(wid, [])
    check("acyclic work-package dependencies", not cycle_errors, "; ".join(cycle_errors))

    text = document.read_text(encoding="utf-8") if document.exists() else ""
    anchors = re.findall(r'<a id="(chapter-\d+)"></a>', text)
    expected_anchors = {f"chapter-{i:02d}" for i in range(1, 41)}
    check("40 unique chapter anchors", set(anchors) == expected_anchors and len(anchors) == 40)
    toc = re.findall(r'^- \[\d{2}[^\]]+\]\(#(chapter-\d+)\)$', text, re.M)
    check("complete table of contents", set(toc) == expected_anchors and len(toc) == 40)
    headings = re.findall(r'^## (\d{2})[　 ]', text, re.M)
    check("40 chapter headings", len(headings) == 40 and len(set(headings)) == 40)
    for label, ids, pattern in (
        ("requirements", reqs, r'\*\*(REQ-\d{3})[｜|]'),
        ("tests", tests, r'^### (T-\d{3})[　 ]'),
        ("work packages", works, r'^### (W-\d{2})[　 ]'),
    ):
        found = re.findall(pattern, text, re.M)
        check("specification " + label + " register", set(found) == set(ids) and len(found) == len(ids))
    adrs = re.findall(r'^\| (ADR-\d{2}) ', text, re.M)
    check("28 decision records", len(adrs) == 28 and set(adrs) == {f"ADR-{i:02d}" for i in range(1, 29)})
    decision_file = root / "docs/decisions/README.md"
    decisions = decision_file.read_text(encoding="utf-8") if decision_file.exists() else ""
    source_rows = re.findall(r'^\| ADR-.*$', text, re.M)
    decision_rows = re.findall(r'^\| ADR-.*$', decisions, re.M)
    check("decision register matches specification", source_rows == decision_rows)

    sources = load("phego-sources.json")
    source_ids = {s.get("id") for s in sources if isinstance(s, dict)} if isinstance(sources, list) else set()
    refs = set(re.findall(r'\[(R\d{2})\]', text))
    check("source references resolve", bool(source_ids) and refs <= source_ids,
          ", ".join(sorted(refs - source_ids)))
    check("source register IDs unique", isinstance(sources, list) and len(sources) == len(source_ids))

    catalog_errors: list[str] = []
    counts = {}
    for name in ("php-history-policy.json", "competitor-catalog.json", "application-catalog.json"):
        obj = load(name)
        if not isinstance(obj, dict) or obj.get("state") != "planning_catalog":
            catalog_errors.append(name + " wrong state")
            continue
        items = obj.get("items", obj.get("required_generations", []))
        ids = [x.get("id") for x in items]
        counts[name] = len(ids)
        if len(set(ids)) != len(ids): catalog_errors.append(name + " duplicate IDs")
        for item in items:
            if item.get("status") != "planned" or item.get("source_lock_status") != "not_locked":
                catalog_errors.append(name + " falsely locked/completed item")
        for rid in obj.get("requirements", []):
            if rid not in reqs: catalog_errors.append(name + " unknown " + rid)
        for wid in obj.get("required_work_packages", []):
            if wid not in works: catalog_errors.append(name + " unknown " + wid)
    check("planning catalogs and references", not catalog_errors, "; ".join(catalog_errors))

    readme_path = root / "README.md"
    readme = readme_path.read_text(encoding="utf-8") if readme_path.exists() else ""
    license_path = root / "LICENSE"
    license_text = license_path.read_text(encoding="utf-8") if license_path.exists() else ""
    check("English README", bool(readme) and not re.search(r'[\u3400-\u9fff]', readme))
    check("requested slogan", "The safest PHP in the world" in readme)
    check("README honest implementation status", "Specification stage" in readme and "not yet contain a runnable PHP engine" in readme)
    check("MIT license identity", license_text.startswith("MIT License\n") and "2026 HazelnutParadise and Phego contributors" in license_text)
    check("MIT license grant and disclaimer", "Permission is hereby granted, free of charge" in license_text and 'THE SOFTWARE IS PROVIDED "AS IS"' in license_text and "SOFTWARE." in license_text)
    check("complete updated scope", all(x in text for x in ["第一版", "全部", "WordPress", "Laravel", "MIT", "HazelnutParadise/phego"]))
    superseded = ["8.3、8.2、7.x 與 5.x 不自動包含在首版承諾中", "首個完整語言認證目標採 **PHP 8.5", "更多 PHP 歷史版本，列為明確的後續能力"]
    check("superseded scope removed", not any(x in text for x in superseded))

    # Validate local Markdown links. External URLs are deliberately not claimed
    # to have been fetched by this offline documentation check.
    link_errors: list[str] = []
    local_link_count = 0
    for md in sorted(root.rglob("*.md")):
        if ".git" in md.parts: continue
        body = md.read_text(encoding="utf-8")
        body = re.sub(r'```.*?```', '', body, flags=re.S)
        for target in re.findall(r'(?<!!)\[[^\]]*\]\(([^\s)]+)\)', body):
            split = urlsplit(target)
            if split.scheme or target.startswith("//"): continue
            local_link_count += 1
            path = (md.parent / unquote(split.path)).resolve() if split.path else md
            if not path.exists():
                link_errors.append(f"{md.relative_to(root)} -> {target}")
            elif split.fragment.startswith("chapter-") and path.is_file():
                if f'id="{split.fragment}"' not in path.read_text(encoding="utf-8"):
                    link_errors.append(f"missing anchor {target}")
    check("local Markdown links", not link_errors, "; ".join(link_errors))

    # Record input hashes without a recursive hash of this output report.
    inputs = {}
    for p in sorted(root.rglob("*")):
        if not p.is_file() or ".git" in p.parts or "__pycache__" in p.parts: continue
        rel = p.relative_to(root).as_posix()
        if rel == "docs/data/document-validation.json": continue
        inputs[rel] = hashlib.sha256(p.read_bytes()).hexdigest()
    return {
        "schema_version": 1, "document_version": "1.1.2", "scope": "documentation_only",
        "status": "pass" if not errors else "fail",
        "counts": {"chapters": len(headings), "requirements": len(reqs), "acceptance_scenarios": len(tests),
                   "work_packages": len(works), "decisions": len(adrs), "sources": len(source_ids),
                   "local_markdown_links": local_link_count, "checks": len(checks), "catalogs": counts},
        "product_test_status": "not_run", "engine_implemented": False,
        "remote_publication_checked": False, "external_urls_checked_by_this_tool": False,
        "checks": checks, "errors": errors, "work_package_topological_order": order,
        "input_sha256": inputs,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--no-write", action="store_true", help="Print results without writing a report.")
    args = parser.parse_args()
    root = args.root.resolve()
    try:
        result = validate(root)
    except (OSError, ValueError, TypeError, KeyError) as exc:
        print(f"Documentation validation could not complete: {exc}", file=sys.stderr)
        return 2
    if not args.no_write:
        out = root / "docs/data/document-validation.json"
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({k: result[k] for k in ("scope", "status", "counts", "product_test_status", "errors")}, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
