#!/usr/bin/env python3
"""W-14 lifecycle oracle. REQ-020/REQ-040; T-039/T-040/T-079/T-080.

Reference output is independent of Phego execution and is never product acceptance.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import uuid

IMAGE = 'php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9'
IMAGES = {
    64: ('linux/arm64', IMAGE),
    32: ('linux/386', 'i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049'),
}
ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / 'tests/lifecycle/w14'
EXPECTED = FIXTURES / 'reference.json'


def collect(width=64):
    platform, image = IMAGES[width]
    inputs = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
              for p in sorted(FIXTURES.glob('*.php'))}
    # Docker acquisition messages are not PHP stderr. Never pull during execution.
    inspected = subprocess.run(['docker', 'image', 'inspect', image], capture_output=True, timeout=30)
    if inspected.returncode:
        subprocess.run(['docker', 'pull', '--platform', platform, image],
                       capture_output=True, timeout=120, check=True)
    name = 'phego-w14-' + uuid.uuid4().hex
    command = ['docker', 'run', '--pull=never', '--platform', platform,
               '--rm', '--name', name, '--network=none', '--read-only',
               '--user=65534:65534',
               '--cap-drop=ALL', '--security-opt=no-new-privileges',
               '--pids-limit=32', '--memory=128m', '--cpus=1',
               '--tmpfs=/tmp:rw,noexec,nosuid,size=8m',
               '--mount', f'type=bind,src={FIXTURES},dst=/fixtures,readonly',
               image, 'php', '-n', '/fixtures/reference.php']
    try:
        # i386 under emulation on an arm64 host needs more than a minute for
        # the 52 fixtures; native runners finish in seconds.
        result = subprocess.run(command, capture_output=True, timeout=300, check=True)
    finally:
        # Killing the Docker client alone does not terminate its container.
        removed = subprocess.run(['docker', 'rm', '-f', name], capture_output=True, timeout=15)
        if removed.returncode and b'No such container' not in removed.stderr:
            raise RuntimeError(f'cannot confirm reference container cleanup: {name}')
    if result.stderr:
        raise ValueError('unexpected reference runner stderr: ' + result.stderr.decode(errors='replace'))
    data = json.loads(result.stdout)
    if (data['php_version'], data['int_size'], data['os']) != ('8.5.12', width // 8, 'Linux'):
        raise ValueError(f'reference profile is not PHP 8.5.12 Linux {width}-bit')
    data['image'] = image
    data['command'] = command
    # The container mount path is stable; host checkout locations are not evidence identity.
    data['command'][data['command'].index('--mount') + 1] = 'type=bind,src=<repo>/tests/lifecycle/w14,dst=/fixtures,readonly'
    data['command'][data['command'].index('--name') + 1] = '<unique-container-name>'
    after = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
             for p in sorted(FIXTURES.glob('*.php'))}
    if inputs != after:
        raise ValueError('fixtures changed during reference execution')
    data['inputs_sha256'] = inputs
    return data


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--width', type=int, choices=(32, 64), default=64,
                        help='Guest integer width: 64 (default, arm64) or 32 (i386).')
    parser.add_argument('--write', action='store_true', help='Create or replace reference.json (64-bit) or reference-32.json after running the locked oracle; default compares without writing.')
    args = parser.parse_args(argv)
    expected = EXPECTED if args.width == 64 else FIXTURES / 'reference-32.json'
    try:
        previous = expected.read_bytes() if expected.exists() else None
        data = collect(args.width)
        if args.write:
            if (expected.read_bytes() if expected.exists() else None) != previous:
                raise ValueError(f'{expected.name} changed during reference execution')
            expected.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
        elif data != json.loads(expected.read_text()):
            raise ValueError(f'reference differs from {expected.name}; inspect the actual streams before updating expectations')
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as exc:
        print(f'lifecycle reference failed: {exc}', file=sys.stderr)
        return 1
    print(f'PASS: {len(data["cases"])} locked PHP lifecycle reference cases ({args.width}-bit)')
    return 0


if __name__ == '__main__':
    sys.exit(main())
