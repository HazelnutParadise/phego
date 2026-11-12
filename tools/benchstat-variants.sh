#!/usr/bin/env bash
# Compares the variants of one benchmark family with benchstat. A family is a
# benchmark whose sub-benchmarks are named <workload>/<variant>, or just
# <variant>; every variant runs the same workload, so one test binary holds
# all sides of the comparison. The binary runs the whole family once per
# round, so load changes on the machine reach every variant alike.
#
# Usage: benchstat-variants.sh PACKAGE FAMILY ROUNDS OUT_DIR
#   FAMILY  benchmark function name, for example BenchmarkVMDispatch
# Needs go and benchstat on PATH. Writes OUT_DIR/FAMILY.<variant>.txt and
# OUT_DIR/FAMILY.benchstat.txt, with the first variant that ran as the base.
set -euo pipefail
package=$1 family=$2 rounds=$3 out=$4
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"
out=$(cd "$out" && pwd)
CGO_ENABLED=0 go test -c -o "$work/bench.test" "$package"
dir=$(go list -f '{{.Dir}}' "$package")
for ((round = 1; round <= rounds; round++)); do
  (cd "$dir" && "$work/bench.test" -test.run '^$' -test.bench "^$family\$" -test.benchmem -test.count 1) >> "$work/raw.txt"
done
# Move the variant, the last name element, out of each benchmark name and
# into the file the line is written to.
variants=$(awk -v family="$family" -v out="$out" '
  $1 ~ "^" family "/" {
    name = $1; cpu = ""
    if (match(name, /-[0-9]+$/)) { cpu = substr(name, RSTART); name = substr(name, 1, RSTART - 1) }
    n = split(name, parts, "/"); variant = parts[n]
    base = parts[1]; for (i = 2; i < n; i++) base = base "/" parts[i]
    $1 = base cpu
    file = out "/" family "." variant ".txt"
    if (!(variant in seen)) { seen[variant] = 1; order[++count] = variant; printf "" > file }
    print >> file
    next
  }
  /^(goos|goarch|pkg|cpu):/ { header = header $0 "\n" }
  END { for (i = 1; i <= count; i++) print order[i]; printf "%s", header > (out "/" family ".header.txt") }
' "$work/raw.txt")
args=()
for variant in $variants; do
  file="$out/$family.$variant.txt"
  { cat "$out/$family.header.txt"; cat "$file"; } > "$work/merged.txt" && mv "$work/merged.txt" "$file"
  args+=("$variant=$file")
done
rm -f "$out/$family.header.txt"
{ echo "# commit $(git rev-parse HEAD)"; echo "# $(go version)"; } > "$out/$family.system.txt"
benchstat "${args[@]}" | tee "$out/$family.benchstat.txt"
