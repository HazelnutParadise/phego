#!/bin/bash
# PHP 8.5.12 behaviour for FIFOs, devices, sockets and directories on the
# running Unix system, for comparison with Phego's Root mounts (W-20).
# Usage: probe.sh PHP WORKDIR PROBE_PHP
#   PHP        php CLI binary
#   WORKDIR    empty directory on a local file system that supports FIFOs
#   PROBE_PHP  path of probe.php
# Each case prints its PHP output and exit status; status 142 means the
# command was still waiting after 3 seconds (SIGALRM).
set -u
php_bin=$1
work=$2
probe=$3
cd "$work" || exit 1
P="$php_bin -n -d display_errors=stderr -d error_reporting=-1"
# limit SECONDS CMD...: run CMD, killing it after SECONDS with status 142.
limit() {
  perl -e 'my $s = shift; my $pid = fork; defined $pid or die "fork: $!";
    if (!$pid) { exec @ARGV or die "exec: $!" }
    $SIG{ALRM} = sub { kill "KILL", $pid; waitpid $pid, 0; exit 142 };
    alarm $s; waitpid $pid, 0; exit($? & 127 ? 128 + ($? & 127) : $? >> 8)' "$@"
}
run() { echo "== $1"; shift; limit 3 "$@" 2>&1; echo "exit=$?"; }
echo "== system: $(uname -sm)"
$P -r 'echo PHP_VERSION, " ", PHP_OS, "\n";'
mkfifo f1 f2 f3 f4 f5 f6 f7 f8 f9 f10 f11 f12 f13 f14
run "fifo r, no writer" $P "$probe" r "$work/f1" read
( sleep 1; exec 3>"$work/f2"; printf payload >&3 ) &
run "fifo r, writer opens at 1s and writes" $P "$probe" r "$work/f2" read
wait
( exec 3>"$work/f3"; sleep 1; printf payload >&3; sleep 1 ) &
run "fifo r, writer already waiting, writes at 1s, closes at 2s" $P "$probe" r "$work/f3" read
wait
( sleep 1; exec 3>"$work/f4"; exec 3>&- ) &
run "fifo r, writer opens and closes at 1s without data" $P "$probe" r "$work/f4" read
wait
run "fifo r+, no peer" $P "$probe" r+ "$work/f5"
run "fifo w, no reader" $P "$probe" w "$work/f6"
( sleep 1; head -c 7 "$work/f7" > "$work/f7.out"; echo "reader got: $(cat "$work/f7.out")" ) &
run "fifo w, reader opens at 1s reads 7 bytes then closes" $P "$probe" w "$work/f7" write
wait
( sleep 1; cat "$work/f8" > /dev/null ) &
run "fifo a, reader at 1s" $P "$probe" a "$work/f8" write
wait
( sleep 1; cat "$work/f9" > /dev/null ) &
run "fifo c, reader at 1s" $P "$probe" c "$work/f9" write
wait
run "fifo x" $P "$probe" x "$work/f10"
( sleep 1; printf '<?php echo "from fifo\\n";' > "$work/f11" ) &
run "php fifo" $P "$work/f11"
wait
( sleep 1; printf '<?php echo "from fifo\\n";' > "$work/f12" ) &
run "include fifo" $P "$probe" include "$work/f12"
wait
( sleep 1; printf 'abc' > "$work/f13" ) &
run "file_get_contents fifo" $P "$probe" fgc "$work/f13"
wait
run "php fifo, no writer" $P "$work/f14"
run "php /dev/tty" $P /dev/tty
run "fopen /dev/tty r" $P "$probe" r /dev/tty
run "fopen /dev/tty w" $P "$probe" w /dev/tty
run "include /dev/tty" $P "$probe" include /dev/tty
run "include /dev/null" $P "$probe" include /dev/null
run "include /dev/zero" $P "$probe" include /dev/zero
run "fopen /dev/ptmx r+" $P "$probe" r+ /dev/ptmx
run "fopen /dev/zero w" $P "$probe" w /dev/zero
mkdir d
run "fopen dir w" $P "$probe" w "$work/d"
run "fopen dir r+" $P "$probe" r+ "$work/d"
run "fopen dir a" $P "$probe" a "$work/d"
$P -r '$s = stream_socket_server("unix://" . $argv[1], $e, $m); sleep(30);' "$work/sock" &
server=$!
sleep 1
run "php socket" $P "$work/sock"
run "fopen socket r" $P "$probe" r "$work/sock"
run "fopen socket w" $P "$probe" w "$work/sock"
run "include socket" $P "$probe" include "$work/sock"
kill $server 2>/dev/null
block=$(ls -l /dev 2>/dev/null | awk '$1 ~ /^b/ {print $NF; exit}')
if [ -n "$block" ]; then
  run "fopen block device /dev/$block r" $P "$probe" r "/dev/$block"
  run "php block device /dev/$block" $P "/dev/$block"
else
  echo "== no block device in /dev"
fi
