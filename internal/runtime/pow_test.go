package runtime

import (
	"bufio"
	"compress/gzip"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func readGzipLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	sc := bufio.NewScanner(z)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func hexBits(t *testing.T, s string) float64 {
	t.Helper()
	u, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		t.Fatal(err)
	}
	return math.Float64frombits(u)
}

// TestGlibcPowMatchesPHP compares every pair of the generated pow corpus (see
// tools/w10_pow_pairs.py) bit for bit with PHP 8.5.12 on 64-bit glibc. It
// includes results exactly halfway between two doubles and inputs where
// glibc is not correctly rounded.
func TestGlibcPowMatchesPHP(t *testing.T) {
	pairs := readGzipLines(t, "testdata/php8512-pow-pairs.txt.gz")
	want := readGzipLines(t, "testdata/php8512-pow-64.txt.gz")
	if dir := os.Getenv(nativeOpsReferenceEnv); dir != "" {
		// The semantics-ops workflow regenerates the results with amd64 PHP.
		raw, err := os.ReadFile(filepath.Join(dir, "pow-64.txt"))
		if err != nil {
			t.Fatal(err)
		}
		want = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	}
	if len(pairs) != len(want) || len(pairs) == 0 {
		t.Fatalf("%d pairs, %d results", len(pairs), len(want))
	}
	failures := 0
	for i, p := range pairs {
		xs, ys, _ := strings.Cut(p, " ")
		x, y := hexBits(t, xs), hexBits(t, ys)
		got := glibcPow(x, y)
		w := hexBits(t, want[i])
		if math.Float64bits(got) != math.Float64bits(w) && !(math.IsNaN(got) && math.IsNaN(w)) {
			if failures++; failures <= 20 {
				t.Errorf("%x ** %x (%v ** %v): php %x, phego %x", math.Float64bits(x), math.Float64bits(y), x, y, math.Float64bits(w), math.Float64bits(got))
			}
		}
	}
	if failures > 0 {
		t.Fatalf("%d of %d pow results differ", failures, len(pairs))
	}
}

// TestGlibcPowKnownCases pins results observed in PHP 8.5.12 on 64-bit glibc.
func TestGlibcPowKnownCases(t *testing.T) {
	for _, tc := range []struct{ x, y, want float64 }{
		{64, 1.5, 512},
		{134217727, 2, 134217727.0 * 134217727.0},
		{0x1.7d914b0bce64ap+13, -0x1.53a701f4c513ep+4, math.Float64frombits(0x2dec09c4a6914016)},
		{0, -1, math.Inf(1)},
		{math.Copysign(0, -1), -1, math.Inf(-1)},
		{-8, 1.0 / 3, math.NaN()},
		{-2, 3, -8},
		{2.3239543184695544, -823.428927093435, math.Float64frombits(0x0152c8b022d050ca)}, // underflow branch
	} {
		got := glibcPow(tc.x, tc.y)
		if math.Float64bits(got) != math.Float64bits(tc.want) && !(math.IsNaN(got) && math.IsNaN(tc.want)) {
			t.Errorf("%v ** %v = %v (%x), want %v (%x)", tc.x, tc.y, got, math.Float64bits(got), tc.want, math.Float64bits(tc.want))
		}
	}
}
