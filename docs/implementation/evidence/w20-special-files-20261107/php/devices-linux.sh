#!/bin/bash
cd /work
P="php -n -d display_errors=stderr -d error_reporting=-1"
run() { echo "== $1"; shift; timeout 3 "$@" 2>&1; echo "exit=$?"; }
mknod /work/chr c 1 3 && run "fopen mknod null(1,3) r" $P /probe/t.php r /work/chr read
mknod /work/blk b 7 0 && { run "fopen block loop0 r" $P /probe/t.php r /work/blk read; run "php block loop0" $P /work/blk; run "include block loop0" $P /probe/t.php include /work/blk; run "fopen block w" $P /probe/t.php w /work/blk; }
mknod /work/blk2 b 7 200 && run "fopen block loop200 (absent) r" $P /probe/t.php r /work/blk2
mknod /work/chr2 c 10 130 && run "fopen watchdog(10,130) not in cgroup r" $P /probe/t.php r /work/chr2
mkfifo /work/nf; run "php fifo no writer" $P /work/nf
mkfifo /work/nf2; run "include fifo no writer" $P /probe/t.php include /work/nf2
