#!/usr/bin/env python3
"""Regenerate the W-17 VM oracle data from the pinned PHP 8.5.12 images.

tables.php prints operation tables as JSON; each snippets/*.php is run as a
program and its stdout (display_errors=1, log_errors=0) and exit code are
recorded. Outputs: tables-64.json, tables-32.json, snippets-64.json,
snippets-32.json next to this script. The images must already be present
locally; nothing is pulled and the containers have no network.
"""
import base64
import json
from pathlib import Path
import subprocess
import sys

IMAGES = {
    64: ('linux/arm64', 'php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9'),
    32: ('linux/386', 'i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049'),
}
HERE = Path(__file__).resolve().parent


def php(width, script):
    platform, image = IMAGES[width]
    cmd = ['docker', 'run', '--rm', '--pull', 'never', '--platform', platform, '--network', 'none', '--cap-drop', 'ALL',
           '-v', f'{HERE}:/w:ro', image, 'php', '-n', '-d', 'display_errors=1', '-d', 'log_errors=0',
           '-d', 'error_reporting=-1', '-d', 'html_errors=0', f'/w/{script}']
    p = subprocess.run(cmd, stdin=subprocess.DEVNULL, capture_output=True, timeout=1800)
    return p.stdout, p.stderr, p.returncode


def main():
    widths = [int(a) for a in sys.argv[1:]] or [64, 32]
    for width in widths:
        out, err, code = php(width, 'tables.php')
        if code != 0 or err:
            sys.exit(f'tables.php failed on {width} bits: {code} {err!r}')
        tables = json.loads(out)
        if tables['php'] != '8.5.12' or tables['int_size'] * 8 != width:
            sys.exit(f'unexpected PHP {tables["php"]} with {tables["int_size"]}-byte ints')
        (HERE / f'tables-{width}.json').write_text(json.dumps(tables, indent=1, sort_keys=True) + '\n')
        snippets = {}
        for path in sorted((HERE / 'snippets').glob('*.php')):
            out, err, code = php(width, f'snippets/{path.name}')
            snippets[path.stem] = {'stdout_b64': base64.b64encode(out).decode(), 'stderr_b64': base64.b64encode(err).decode(), 'exit_code': code}
        (HERE / f'snippets-{width}.json').write_text(json.dumps({'php': '8.5.12', 'width': width, 'image': IMAGES[width][1], 'snippets': snippets}, indent=1, sort_keys=True) + '\n')


if __name__ == '__main__':
    main()
