#!/usr/bin/env python3
"""W-17 differential reference: run every tests/vm/w17/cases/*.php in pinned
PHP 8.5.12 images and record stdout, stderr and exit code per error mode.

The reference is independent of Phego execution and is never product acceptance.
Without --write the reference is regenerated and compared with the committed
profile file; any drift is a failure.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import uuid

# Profiles are (integer width, architecture): the same width can differ by
# architecture, so each pinned image has its own reference file.
IMAGES = {
    (64, 'arm64'): ('linux/arm64', 'php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9'),
    (32, '386'): ('linux/386', 'i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049'),
    (64, 'amd64'): ('linux/amd64', 'php@sha256:35fd7ee3ab5af1ed1f23f6bb01a3464c10ad0bb813a020b29b770115b852d06f'),
}
DEFAULT_ARCH = {64: 'arm64', 32: '386'}
FILES = {
    (64, 'arm64'): 'reference-64.json',
    (32, '386'): 'reference-32.json',
    (64, 'amd64'): 'reference-64-amd64.json',
}
# php_uname('m') reports the kernel's machine name. A linux/386 container on
# an x86_64 kernel reports x86_64 and under emulation it reports i686, so the
# field is checked against this set and is not part of drift comparison.
MACHINES = {
    (64, 'arm64'): {'aarch64'},
    (32, '386'): {'i386', 'i586', 'i686', 'x86_64'},
    (64, 'amd64'): {'x86_64'},
}
HOST_DEPENDENT = ('machine',)
FIXTURES = Path(__file__).resolve().parent
MOUNT = '<repo>/tests/vm/w17'
CONTAINER_NAME = '<unique-container-name>'
# Emulated i386 needs several minutes for every case run three times; native
# runners finish in well under a minute.
RUN_TIMEOUT = 1800


def resolve_profile(width, arch=None):
    """Return the (width, arch) profile key or raise ValueError when no image is pinned."""
    key = (width, arch or DEFAULT_ARCH[width])
    if key not in IMAGES:
        raise ValueError(f'no pinned image for {width}-bit {key[1]}')
    return key


def reference_path(width, arch=None, directory=None):
    return Path(directory or FIXTURES) / FILES[resolve_profile(width, arch)]


def input_hashes(fixtures=None):
    """SHA-256 of every input that shapes the reference: the cases and the runner."""
    fixtures = Path(fixtures or FIXTURES)
    files = sorted(fixtures.glob('cases/*.php')) + [fixtures / 'reference.php']
    return {p.relative_to(fixtures).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in files}


def docker_command(platform, image, name, fixtures):
    return ['docker', 'run', '--pull=never', '--platform', platform,
            '--rm', '--name', name, '--network=none', '--read-only',
            '--user=65534:65534',
            '--cap-drop=ALL', '--security-opt=no-new-privileges',
            '--pids-limit=64', '--memory=256m', '--cpus=1',
            '--tmpfs=/tmp:rw,noexec,nosuid,size=8m',
            '--mount', f'type=bind,src={fixtures},dst=/w,readonly',
            image, 'php', '-n', '/w/reference.php']


def scrub_command(command):
    """Replace the host checkout path and the random container name."""
    scrubbed = list(command)
    scrubbed[scrubbed.index('--mount') + 1] = f'type=bind,src={MOUNT},dst=/w,readonly'
    scrubbed[scrubbed.index('--name') + 1] = CONTAINER_NAME
    return scrubbed


def prepare_image(platform, image):
    # Docker acquisition messages are not PHP output. Never pull during execution.
    inspected = subprocess.run(['docker', 'image', 'inspect', image], capture_output=True, timeout=30)
    if inspected.returncode:
        subprocess.run(['docker', 'pull', '--platform', platform, image],
                       capture_output=True, timeout=600, check=True)


def collect(width=64, arch=None, fixtures=None):
    key = resolve_profile(width, arch)
    fixtures = Path(fixtures or FIXTURES)
    platform, image = IMAGES[key]
    inputs = input_hashes(fixtures)
    prepare_image(platform, image)
    name = 'phego-w17-' + uuid.uuid4().hex
    command = docker_command(platform, image, name, fixtures)
    try:
        result = subprocess.run(command, capture_output=True, timeout=RUN_TIMEOUT, check=True)
    finally:
        # Killing the Docker client alone does not terminate its container.
        removed = subprocess.run(['docker', 'rm', '-f', name], capture_output=True, timeout=30)
        if removed.returncode and b'No such container' not in removed.stderr:
            raise RuntimeError(f'cannot confirm reference container cleanup: {name}')
    if result.stderr:
        raise ValueError('unexpected reference runner stderr: ' + result.stderr.decode(errors='replace'))
    data = json.loads(result.stdout)
    if (data.get('php_version'), data.get('int_size'), data.get('os')) != ('8.5.12', width // 8, 'Linux'):
        raise ValueError(f'reference profile is not PHP 8.5.12 Linux {width}-bit')
    if data.get('machine') not in MACHINES[key]:
        raise ValueError(f'unexpected machine {data.get("machine")!r} for {width}-bit {key[1]}')
    if input_hashes(fixtures) != inputs:
        raise ValueError('inputs changed during reference execution')
    cases = data.pop('cases')
    if [c['case'] + '.php' for c in cases] != [Path(p).name for p in inputs if p.startswith('cases/')]:
        raise ValueError('reference cases do not match the case files')
    for case in cases:
        if case['source_sha256'] != inputs['cases/' + case['case'] + '.php']:
            raise ValueError(f'case {case["case"]} hash differs from its input')
    data.update({'image': image, 'platform': platform, 'command': scrub_command(command),
                 'inputs_sha256': inputs, 'cases': cases})
    return data


def drift(expected, actual):
    """Describe every difference that matters; an empty list means no drift."""
    problems = []
    for key in sorted(set(expected) | set(actual)):
        if key in HOST_DEPENDENT or key == 'cases':
            continue
        if expected.get(key) != actual.get(key):
            problems.append(f'top-level field {key} differs')
    old = {c['case']: c for c in expected.get('cases', [])}
    new = {c['case']: c for c in actual.get('cases', [])}
    for name in sorted(set(old) | set(new)):
        if name not in new:
            problems.append(f'case {name} is missing from the regenerated reference')
        elif name not in old:
            problems.append(f'case {name} is not in the committed reference')
        elif old[name] != new[name]:
            fields = sorted(k for k in set(old[name]) | set(new[name]) if old[name].get(k) != new[name].get(k))
            problems.append(f'case {name} differs in {", ".join(fields)}')
    if [c['case'] for c in expected.get('cases', [])] != [c['case'] for c in actual.get('cases', [])] and not any(
            p.startswith('case ') for p in problems):
        problems.append('case order differs')
    return problems


def encode(data):
    return json.dumps(data, ensure_ascii=False, indent=2) + '\n'


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='Example: python3 tests/vm/w17/reference.py --width 32 --write')
    parser.add_argument('--width', type=int, choices=(32, 64), default=64,
                        help='Guest integer width: 64 (default, arm64) or 32 (386).')
    parser.add_argument('--arch', choices=('arm64', '386', 'amd64'),
                        help='Guest architecture: arm64 (default for 64), 386 (default for 32) or amd64.')
    parser.add_argument('--write', action='store_true',
                        help='Create or replace the profile reference (reference-64.json, reference-32.json or '
                             'reference-64-amd64.json); default regenerates and compares without writing.')
    parser.add_argument('--output-dir', type=Path,
                        help='With --write, write the profile file here instead of tests/vm/w17; without --write, '
                             'also save the regenerated reference here for inspection.')
    args = parser.parse_args(argv)
    try:
        args.profile = resolve_profile(args.width, args.arch)
    except ValueError as exc:
        parser.error(str(exc))
    return args


def main(argv=None):
    args = parse_args(argv)
    width, arch = args.profile
    committed = reference_path(width, arch)
    target = reference_path(width, arch, args.output_dir) if args.output_dir else committed
    try:
        previous = target.read_bytes() if target.exists() else None
        data = collect(width, arch)
        if args.write:
            if (target.read_bytes() if target.exists() else None) != previous:
                raise ValueError(f'{target.name} changed during reference execution')
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(encode(data))
        else:
            if args.output_dir:
                args.output_dir.mkdir(parents=True, exist_ok=True)
                (args.output_dir / committed.name).write_text(encode(data))
            problems = drift(json.loads(committed.read_text()), data)
            if problems:
                raise ValueError(f'reference differs from {committed.name}; inspect the actual streams before '
                                 'updating expectations:\n  ' + '\n  '.join(problems))
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as exc:
        print(f'w17 reference failed: {exc}', file=sys.stderr)
        return 1
    print(f'PASS: {len(data["cases"])} W-17 reference cases ({width}-bit {arch}, '
          f'{"written to " + target.name if args.write else "matches " + committed.name})')
    return 0


if __name__ == '__main__':
    sys.exit(main())
