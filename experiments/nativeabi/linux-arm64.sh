#!/usr/bin/env bash
# Reproduce the W-07 numeric probe with the W-02 pinned Linux arm64 toolchains.
set -euo pipefail
if [[ $# != 2 || ${1-} == --help ]]; then
  printf 'Usage: bash experiments/nativeabi/linux-arm64.sh LLVM_ARCHIVE GO_ARCHIVE\n' >&2
  printf 'Archives must match the Linux arm64 hashes in compat.lock.json.\n' >&2
  [[ ${1-} == --help ]] && exit 0
  exit 2
fi
llvm_archive=$1
go_archive=$2
source_dir=$(cd -- "$(dirname -- "$0")" && pwd -P)
for path in "$llvm_archive" "$go_archive" "$source_dir"; do
  if [[ "$path" != /* || "$path" == *,* ]]; then
    printf 'error: paths must be absolute and contain no comma: %s\n' "$path" >&2
    exit 2
  fi
done
printf '%s  %s\n' '0975f753e0f9b5f83146a7a2f4678bc143e713496b56520716a4de047a63f99f' "$llvm_archive" | shasum -a 256 -c -
printf '%s  %s\n' 'a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49' "$go_archive" | shasum -a 256 -c -
zstd --long=30 -dc "$llvm_archive" | docker run --rm -i --platform linux/arm64 --network none --read-only --user 65534:65534 --pids-limit 128 --memory 3g --cpus 2 --cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,exec,nosuid,size=2g --mount "type=bind,src=$go_archive,dst=/input/go.tar.gz,readonly" --mount "type=bind,src=$source_dir,dst=/src,readonly" buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3 sh -ec '
  tar -xf - -C /tmp LLVM-23.1.4-Linux-ARM64/bin/clang LLVM-23.1.4-Linux-ARM64/bin/clang-23 LLVM-23.1.4-Linux-ARM64/lib/clang/23/include
  tar -xzf /input/go.tar.gz -C /tmp
  /tmp/LLVM-23.1.4-Linux-ARM64/bin/clang --version | head -1
  /tmp/go/bin/go version
  /tmp/LLVM-23.1.4-Linux-ARM64/bin/clang -c /src/xor.ll -o /tmp/probe.o
  ar rcs /tmp/libphegoprobe.a /tmp/probe.o
  HOME=/tmp GOPATH=/tmp/gopath GOCACHE=/tmp/gocache GO111MODULE=off CGO_ENABLED=1 CC=/tmp/LLVM-23.1.4-Linux-ARM64/bin/clang CGO_LDFLAGS="-L/tmp -lphegoprobe" /tmp/go/bin/go run -tags=phego_native_probe /src/main.go
'
