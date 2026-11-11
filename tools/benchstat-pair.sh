#!/usr/bin/env bash
# Compares benchmarks of ./internal/runtime between two commits with
# benchstat. Each commit is exported and built into its own test binary; the
# two binaries then run alternately, one sample per round, so that load
# changes on the machine reach both sides alike.
#
# Usage: benchstat-pair.sh NAME BEFORE AFTER BENCH_REGEX ROUNDS OUT_DIR [BENCH_FILE...]
#   BENCH_FILE  benchmark source files copied from AFTER into BEFORE, for
#               benchmarks that were added together with the change
# Needs git, go and benchstat on PATH. Writes OUT_DIR/NAME.{before,after,benchstat}.txt.
set -euo pipefail
name=$1 before=$2 after=$3 bench=$4 rounds=$5 out=$6
shift 6
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"
out=$(cd "$out" && pwd)
for side in before after; do
  mkdir "$work/$side"
  git archive "${!side}" | tar -x -C "$work/$side"
done
for file in "$@"; do
  cp "$work/after/$file" "$work/before/$file"
done
for side in before after; do
  (cd "$work/$side" && CGO_ENABLED=0 go test -c -o "$work/$side.test" ./internal/runtime)
  # Not "key: value" lines: benchstat reads those as configuration and puts
  # files whose commits differ into separate tables instead of comparing them.
  {
    echo "# commit $(git rev-parse "${!side}")"
    echo "# $(go version)"
  } > "$out/$name.$side.txt"
done
for ((round = 1; round <= rounds; round++)); do
  for side in before after; do
    (cd "$work/$side/internal/runtime" && "$work/$side.test" -test.run '^$' -test.bench "$bench" -test.benchmem -test.count 1) >> "$out/$name.$side.txt"
  done
done
benchstat "before=$out/$name.before.txt" "after=$out/$name.after.txt" | tee "$out/$name.benchstat.txt"
