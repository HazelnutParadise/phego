#!/usr/bin/env python3
"""Check execution records and whether their source commit exists locally.

A commit's existence does not prove that the command ran or that its result is
correct. This tool does not certify a work package or product acceptance.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys

import traceability


def _git(root: Path, *args: str) -> str:
    env = {key: value for key, value in os.environ.items() if not key.upper().startswith('GIT_')}
    env['GIT_NO_REPLACE_OBJECTS'] = '1'
    env['GIT_TERMINAL_PROMPT'] = '0'
    try:
        proc = subprocess.run(['git', '-C', str(root), *args], env=env,
                              stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, text=True, timeout=15, check=False)
    except (OSError, subprocess.TimeoutExpired, UnicodeError) as exc:
        raise ValueError(f'cannot inspect repository: {exc}') from exc
    if proc.returncode != 0:
        raise ValueError('Git repository or recorded commit is unavailable')
    return proc.stdout.strip()


def verify(root: Path, results: Path | None) -> dict:
    """Validate records, then check the exact commit object in the given Git root."""
    try:
        root = root.resolve(strict=True)
    except OSError as exc:
        raise ValueError(f'project root is unavailable: {root}') from exc
    if not root.is_dir():
        raise ValueError('project root must be a directory')
    top = _git(root, 'rev-parse', '--show-toplevel')
    if Path(top).resolve() != root:
        raise ValueError('project root must be the Git worktree root')
    output = traceability.report(root, results)
    commit = output['commit']
    if commit is None:
        output['commit_check'] = 'not_applicable'
    else:
        if _git(root, 'cat-file', '-t', commit) != 'commit':
            raise ValueError('recorded object is not a commit')
        output['commit_check'] = 'exists'
    output['execution_verification'] = 'not_verified'
    return output


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog=(
        'Example: python3 tools/check_evidence_commit.py --results records.json. '
        'Invalid input, missing Git objects or non-commit objects exit 2.'))
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1],
                        help='Git worktree root (default: repository containing this tool)')
    parser.add_argument('--results', type=Path,
                        help='execution records JSON (omitted tests remain not_run)')
    args = parser.parse_args()
    try:
        output = verify(args.root, args.results)
    except (OSError, TypeError, ValueError) as exc:
        print(f'check_evidence_commit: error: {exc}', file=sys.stderr)
        return 2
    print(json.dumps(output, ensure_ascii=False, sort_keys=True, indent=2))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
