#!/bin/bash
set -euo pipefail
archive=${1:?usage: bash repro.sh /path/to/LLVM-23.1.4-Linux-X64.tar.zst}
expected=efdd1dbd089b99bfcfcde7b8eb1236163f2e88df5a981cca4f783ec9cb5e0378
gate=$(mktemp -d "${TMPDIR:-/tmp}/phego-llvm-gate.XXXXXXXX")
chmod 755 "$gate"
trap 'rm -rf -- "$gate"' EXIT
tee >(shasum -a 256 > "$gate/digest"; if [[ $(cut -d' ' -f1 "$gate/digest") == "$expected" ]]; then touch "$gate/verified"; else touch "$gate/rejected"; fi) < "$archive" |
  zstd -dc --long=30 |
  docker run --rm -i --platform linux/amd64 --network none --cap-drop ALL --security-opt no-new-privileges --read-only --cpus 2 --memory 2g --pids-limit 128 --tmpfs /tmp:exec,size=1g --mount "type=bind,src=$gate,dst=/gate,readonly" buildpack-deps@sha256:0766bfc10a5b87c895c6f675bb8dd09067821b4de144b047a6d029e0c366ecac timeout 600 bash -euc '
    cd /tmp
    tar --no-same-owner -xf - LLVM-23.1.4-Linux-X64/bin/clang LLVM-23.1.4-Linux-X64/bin/clang-23 LLVM-23.1.4-Linux-X64/bin/ld.lld LLVM-23.1.4-Linux-X64/bin/lld
    for i in $(seq 1 1200); do
      test ! -e /gate/rejected || { echo stream_digest_mismatch >&2; exit 1; }
      test ! -e /gate/verified || break
      sleep 1
    done
    test -e /gate/verified || { echo stream_digest_timeout >&2; exit 1; }
    echo VERIFIED_BEFORE_EXECUTION
    uname -m
    ldd --version | head -n 1
    /tmp/LLVM-23.1.4-Linux-X64/bin/clang --version
    /tmp/LLVM-23.1.4-Linux-X64/bin/ld.lld --version
    printf "int main(void) { return 0; }\n" > /tmp/probe.c
    /tmp/LLVM-23.1.4-Linux-X64/bin/clang -fuse-ld=lld /tmp/probe.c -o /tmp/probe
    /tmp/probe
    sha256sum /tmp/probe
    echo LLVM_GATED_AMD64_PROBE_OK
  '
