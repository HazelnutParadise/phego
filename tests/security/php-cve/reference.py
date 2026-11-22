#!/usr/bin/env python3
"""PHP CVE regression corpus reference: run every tests/security/php-cve/cases/*.php
in the pinned PHP 8.5.12 images and record stdout, stderr and exit code per
error mode, exactly as tests/vm/w17/reference.py does for the VM fixtures.

The runner is the W-17 one: this script stages the cases next to a copy of
tests/vm/w17/reference.php and calls that module, so both corpora share one
oracle. The reference is never product acceptance. Without --write the
reference is regenerated and compared with the committed profile file; any
drift is a failure.
"""
import argparse
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

HERE = Path(__file__).resolve().parent
W17 = HERE.parent.parent / 'vm' / 'w17'


def load_w17():
    spec = importlib.util.spec_from_file_location('w17_reference', W17 / 'reference.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def collect(w17, width, arch):
    """Stage cases/ and the W-17 runner in one directory and collect from it."""
    with tempfile.TemporaryDirectory(prefix='phego-cve-ref-') as tmp:
        staging = Path(tmp)
        # The container runs as nobody (65534); TemporaryDirectory is 0700.
        staging.chmod(0o755)
        shutil.copytree(HERE / 'cases', staging / 'cases')
        shutil.copyfile(W17 / 'reference.php', staging / 'reference.php')
        for path in [staging / 'cases', *staging.rglob('*')]:
            path.chmod(0o755 if path.is_dir() else 0o644)
        data = w17.collect(width, arch, fixtures=staging)
    # The W-17 scrubber names its own directory as the mount source; this run
    # mounted a staging copy of cases/ plus the W-17 reference.php.
    mount = data['command'].index('--mount') + 1
    data['command'][mount] = 'type=bind,src=<staging: tests/security/php-cve/cases + tests/vm/w17/reference.php>,dst=/w,readonly'
    if w17.input_hashes(HERE.parent.parent / 'vm' / 'w17')['reference.php'] != data['inputs_sha256']['reference.php']:
        raise ValueError('tests/vm/w17/reference.php changed during reference execution')
    return data


def main(argv=None):
    w17 = load_w17()
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter,
                                     epilog='Example: python3 tests/security/php-cve/reference.py --width 32 --write')
    parser.add_argument('--width', type=int, choices=(32, 64), default=64,
                        help='Guest integer width: 64 (default, arm64) or 32 (386).')
    parser.add_argument('--arch', choices=('arm64', '386', 'amd64'),
                        help='Guest architecture: arm64 (default for 64), 386 (default for 32) or amd64.')
    parser.add_argument('--write', action='store_true',
                        help='Create or replace the profile reference; default regenerates and compares without writing.')
    parser.add_argument('--output-dir', type=Path,
                        help='Without --write, also save the regenerated reference here for inspection.')
    args = parser.parse_args(argv)
    try:
        width, arch = w17.resolve_profile(args.width, args.arch)
    except ValueError as exc:
        parser.error(str(exc))
    committed = w17.reference_path(width, arch, HERE)
    try:
        previous = committed.read_bytes() if committed.exists() else None
        data = collect(w17, width, arch)
        if args.write:
            if (committed.read_bytes() if committed.exists() else None) != previous:
                raise ValueError(f'{committed.name} changed during reference execution')
            committed.write_text(w17.encode(data))
        else:
            if args.output_dir:
                args.output_dir.mkdir(parents=True, exist_ok=True)
                (args.output_dir / committed.name).write_text(w17.encode(data))
            problems = w17.drift(json.loads(committed.read_text()), data)
            if problems:
                raise ValueError(f'reference differs from {committed.name}; inspect the actual streams before '
                                 'updating expectations:\n  ' + '\n  '.join(problems))
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as exc:
        print(f'php-cve reference failed: {exc}', file=sys.stderr)
        return 1
    print(f'PASS: {len(data["cases"])} PHP CVE reference cases ({width}-bit {arch}, '
          f'{"written to " + committed.name if args.write else "matches " + committed.name})')
    return 0


if __name__ == '__main__':
    sys.exit(main())
