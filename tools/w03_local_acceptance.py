#!/usr/bin/env python3
"""Evaluate W-03 local exit from checks executed in this invocation."""
from __future__ import annotations

import argparse
import contextlib
import io
import importlib.util
import os
import json
from pathlib import Path
import re
import subprocess
import tempfile
import sys



REQUIRED_CASES = frozenset({
    'test_work_status.WorkStatusTest.test_requirement_with_missing_test_is_rejected',
    'test_work_status.WorkStatusTest.test_dependency_cycle_in_plan_is_rejected',
    'test_w03_local_gate.ObservedExecutionTests.test_missing_or_empty_or_duplicate_cases_are_rejected',
    'test_w03_local_gate.ObservedExecutionTests.test_failure_and_skip_are_not_pass',
    'test_w03_gate_cli.LocalGateCLITests.test_imported_results_are_not_accepted',
    'test_w03_local_acceptance.LocalExitTests.test_completed_claim_must_have_matching_finished_observation',
})


def observed_local_exit(report: object, exit_code: int, commit: str) -> bool:
    """Accept only a complete observation from the current local execution."""
    if exit_code != 0 or not isinstance(report, dict) or report.get('commit') != commit:
        return False
    if report.get('package_acceptance') != 'not_claimed' or report.get('product_acceptance') != 'not_claimed':
        return False
    observation = report.get('observation')
    if not isinstance(observation, dict) or observation.get('status') != 'pass':
        return False
    cases, events, count = observation.get('cases'), observation.get('events'), report.get('expected_count')
    if (not isinstance(cases, dict) or not isinstance(events, list) or type(count) is not int
            or count != len(cases) or not REQUIRED_CASES.issubset(cases)
            or len(events) != 3 * count or any(v != 'pass' for v in cases.values())):
        return False
    seen = set()
    for index in range(0, len(events), 3):
        start, outcome, stop = events[index:index + 3]
        if not all(isinstance(event, list) and len(event) == 2 for event in (start, outcome, stop)):
            return False
        name = start[0]
        if (not isinstance(name, str) or name not in cases or name in seen
                or start != [name, 'start'] or outcome != [name, 'pass'] or stop != [name, 'stop']):
            return False
        seen.add(name)
    return seen == set(cases)


def committed_blob(root: Path, commit: str, relative: str) -> bytes:
    """Read a bounded blob using Git without importing worktree code."""
    env = {k: v for k, v in os.environ.items() if not k.upper().startswith('GIT_')}
    env.update({'GIT_NO_REPLACE_OBJECTS': '1', 'GIT_CONFIG_NOSYSTEM': '1',
                'GIT_CONFIG_GLOBAL': os.devnull, 'GIT_TERMINAL_PROMPT': '0'})
    ref = commit + ':' + relative
    def git(*arguments: str) -> bytes:
        try:
            result = subprocess.run(['git', '-C', str(root), *arguments], env=env,
                                    stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE, timeout=20, check=False)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise ValueError('cannot inspect committed tool') from exc
        if result.returncode:
            raise ValueError('committed tool is unavailable')
        return result.stdout
    try:
        size = int(git('cat-file', '-s', ref).strip())
    except ValueError as exc:
        raise ValueError('invalid committed tool size') from exc
    if size < 0 or size > 1024 * 1024:
        raise ValueError('committed tool exceeds size limit')
    content = git('cat-file', 'blob', ref)
    if len(content) != size:
        raise ValueError('committed tool size changed')
    return content


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog=(
        'Example: python3 tools/w03_local_acceptance.py --commit <full-commit-sha>. '
        'Executes trusted committed checks; this is not a security sandbox or product certification.'))
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1], help='Git worktree root')
    parser.add_argument('--commit', required=True, help='full lowercase SHA-1 of the trusted source commit')
    parser.add_argument('--timeout', type=int, default=1800, help='suite timeout in seconds (1-3600; default 1800)')
    args = parser.parse_args()
    if not re.fullmatch(r'[0-9a-f]{40}', args.commit) or not 1 <= args.timeout <= 3600:
        parser.error('use a full lowercase SHA-1 and a timeout between 1 and 3600')
    try:
        root = args.root.resolve(strict=True)
        committed = committed_blob(root, args.commit, 'tools/w03_local_acceptance.py')
        if committed != Path(__file__).read_bytes():
            raise ValueError('running local acceptance tool differs from selected commit')
        gate_source = committed_blob(root, args.commit, 'tools/w03_local_gate.py')
        output = io.StringIO()
        previous_argv = sys.argv
        with tempfile.TemporaryDirectory(prefix='phego-w03-acceptance-') as temporary:
            gate_path = Path(temporary) / 'w03_local_gate.py'
            gate_path.write_bytes(gate_source)
            spec = importlib.util.spec_from_file_location('phego_committed_w03_gate', gate_path)
            if spec is None or spec.loader is None:
                raise ValueError('cannot load committed execution tool')
            gate = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(gate)
            try:
                sys.argv = [str(gate_path), '--root', str(root), '--commit', args.commit, '--timeout', str(args.timeout)]
                with contextlib.redirect_stdout(output):
                    exit_code = gate.main()
            finally:
                sys.argv = previous_argv
        try:
            report = json.loads(output.getvalue())
        except json.JSONDecodeError:
            report = None
        passed = observed_local_exit(report, exit_code, args.commit)
        result = {
            'local_exit_gate': {'work_package': 'W-03', 'scope': 'local_tools_only',
                                'status': 'pass' if passed else 'fail', 'basis': 'observed_execution'},
            'package_acceptance': 'not_claimed', 'product_acceptance': 'not_claimed',
            'observation_report': report,
        }
        print(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2))
        return 0 if passed else 1
    except (OSError, ValueError, UnicodeError) as exc:
        print('w03_local_acceptance: error: ' + str(exc), file=sys.stderr)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
