package runtime

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestI386PowMatchesPHP(t *testing.T) {
	pairs := readGzipLines(t, "testdata/php8512-pow-pairs.txt.gz")
	want := readGzipLines(t, "testdata/php8512-pow-32.txt.gz")
	if dir := os.Getenv(nativeOpsReferenceEnv); dir != "" {
		raw, err := os.ReadFile(filepath.Join(dir, "pow-32.txt"))
		if err != nil {
			t.Fatal(err)
		}
		want = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	}
	if len(pairs) != len(want) || len(pairs) == 0 {
		t.Fatalf("%d pairs, %d results", len(pairs), len(want))
	}
	failures := 0
	for i, pair := range pairs {
		x, y, _ := strings.Cut(pair, " ")
		got, expected := i386Pow(hexBits(t, x), hexBits(t, y)), hexBits(t, want[i])
		if math.Float64bits(got) != math.Float64bits(expected) && !(math.IsNaN(got) && math.IsNaN(expected)) {
			failures++
			if failures <= 10 {
				t.Errorf("%s ** %s: got %016x want %s", x, y, math.Float64bits(got), want[i])
			}
		}
	}
	if failures != 0 {
		t.Fatalf("%d of %d results differ", failures, len(pairs))
	}
}

func TestI386PowArithmeticOrder(t *testing.T) {
	for _, tc := range []struct {
		x, y float64
		bits uint64
	}{
		{7, -7, 0x3eb45f3b3bb08298},
		{1e20, .5, 0x4202a05f20000004},
		{1e100, -1.5, 0x20ca2fe76a3f944c},
		{63, 64, 0x57d75bf07046ee47},
	} {
		if got := i386Pow(tc.x, tc.y); math.Float64bits(got) != tc.bits {
			t.Errorf("%g ** %g: got %016x want %016x", tc.x, tc.y, math.Float64bits(got), tc.bits)
		}
	}
	h, err := newHeap(32)
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.arith("**", h.number(114), h.number(23))
	if err != nil || v.kind != valueFloat || math.Float64bits(v.floating) != 0x49c1d539a928cbc9 {
		t.Fatalf("integer overflow chain: value=%+v err=%v", v, err)
	}
}
