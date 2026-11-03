package bytecode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"
)

func FuzzLoadRejectsMalformedAndRepairedPayloads(f *testing.F) {
	seedProgram, err := Build(context.Background(), validImage(), testOptions())
	if err != nil || seedProgram == nil {
		f.Fatalf("cannot construct valid bytecode fuzz seed: Program=%v err=%v", seedProgram, err)
	}
	encodedSeed, err := seedProgram.Encode(context.Background())
	if err != nil || len(encodedSeed) < 64 {
		f.Fatalf("cannot encode valid bytecode fuzz seed: len=%d err=%v", len(encodedSeed), err)
	}
	f.Add(append([]byte(nil), encodedSeed[64:]...))
	f.Add(append([]byte(nil), encodedSeed...))
	f.Add([]byte{})
	f.Add([]byte("PHEGOBC\x00"))
	f.Add(make([]byte, 64))
	f.Add([]byte{0xff, 0, 1, 2, 3, 4, 5, 6, 7})
	f.Fuzz(func(t *testing.T, input []byte) {
		limits := DefaultLimits()
		limits.MaxBytes = 4096
		limits.MaxMemory = 1 << 20
		limits.MaxWork = 1 << 20
		opts := LoadOptions{Options: Options{Limits: limits, Supported: AllCapabilities}, Expected: testIdentity()}
		check := func(data []byte, label string) {
			t.Helper()
			o := opts
			if len(data) <= int(o.Limits.MaxBytes) {
				o.TrustedDigest = sha256.Sum256(data)
			}
			p, err := Load(context.Background(), data, o)
			if err != nil && p != nil {
				t.Fatalf("%s returned partial Program: %v", label, p)
			}
			if len(data) > int(o.Limits.MaxBytes) {
				if !errors.Is(err, ErrLimit) {
					t.Fatalf("%s length %d returned error %v, want ErrLimit", label, len(data), err)
				}
				return
			}
			if err != nil {
				return
			}
			if p == nil {
				t.Fatalf("%s succeeded with nil Program", label)
			}
			canonical, err := p.Encode(context.Background())
			if err != nil {
				t.Fatalf("%s Encode after Load: %v", label, err)
			}
			reloaded, err := Load(context.Background(), canonical, LoadOptions{Options: opts.Options, Expected: opts.Expected, TrustedDigest: sha256.Sum256(canonical)})
			if err != nil || reloaded == nil {
				t.Fatalf("%s canonical reload=(%v,%v)", label, reloaded, err)
			}
			reencoded, err := reloaded.Encode(context.Background())
			if err != nil || !bytes.Equal(canonical, reencoded) {
				t.Fatalf("%s canonical bytes changed after reload: err=%v equal=%v", label, err, bytes.Equal(canonical, reencoded))
			}
		}
		// First exercise the full raw-input gate, including the size limit.
		check(append([]byte(nil), input...), "raw")
		// Then make a bounded, header-valid envelope around arbitrary bytes so
		// the payload parser receives fuzzed data with a repaired internal hash.
		payload := append([]byte(nil), input...)
		if len(payload) > 4032 {
			payload = payload[:4032]
		}
		repaired := make([]byte, 64+len(payload))
		copy(repaired[:8], []byte{'P', 'H', 'E', 'G', 'O', 'B', 'C', 0})
		binary.LittleEndian.PutUint16(repaired[8:10], 1)
		binary.LittleEndian.PutUint16(repaired[10:12], 0)
		binary.LittleEndian.PutUint32(repaired[12:16], 64)
		binary.LittleEndian.PutUint64(repaired[16:24], uint64(len(repaired)))
		copy(repaired[64:], payload)
		repairWireIntegrity(repaired)
		check(repaired, "repaired payload")
	})
}
