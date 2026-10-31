package runtime

import (
	"math"
	"testing"
)

// FuzzNumericString checks that strict recognition is a subset of lenient
// recognition with the same value, and that formatting never panics.
func FuzzNumericString(f *testing.F) {
	for _, s := range []string{"", "0", " 1", "1 ", "1abc", "1e3", ".5", "-0", "9223372036854775808",
		"-9223372036854775808", "2147483648", "1e", "1e+", "0x1A", "\x001", "1\x00", "  \t\n", "+.5e-3x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, width := range []int{32, 64} {
			strict := php8512NumericString(s, width, false)
			lenient := php8512NumericString(s, width, true)
			if strict.kind != notNumeric {
				if lenient.kind != strict.kind || lenient.trailing || lenient.integer != strict.integer ||
					math.Float64bits(lenient.floating) != math.Float64bits(strict.floating) {
					t.Fatalf("%q width %d: strict %+v lenient %+v", s, width, strict, lenient)
				}
			}
			if lenient.kind == numericInt && (width == 32 && (lenient.integer > math.MaxInt32 || lenient.integer < math.MinInt32)) {
				t.Fatalf("%q: integer %d outside 32-bit range", s, lenient.integer)
			}
			_ = php8512FormatFloat(lenient.floating, 14)
			_ = php8512FormatFloat(lenient.floating, -1)
		}
	})
}

// TestNumericStringExponentSignQuirk pins PHP 8.5.12 results observed in the
// pinned images: after "e+" or "e-" without digits, PHP's overflow check
// compares the digit run shifted by one byte, so an oversized run wraps.
func TestNumericStringExponentSignQuirk(t *testing.T) {
	for _, tc := range []struct {
		s     string
		width int
		kind  numericKind
		i     int64
	}{
		{"7000000000E+", 32, numericInt, -1589934592},
		{"7000000000E-x", 32, numericInt, -1589934592},
		{"2147483648E+", 32, numericInt, -2147483648},
		{"2147483647E+", 32, numericInt, 2147483647},
		{"7000000000e", 32, numericFloat, 0},
		{"9999999999e+", 32, numericFloat, 0},
		{"9223372036854775808E+", 64, numericInt, math.MinInt64},
		{"9993372036854775808e+", 64, numericFloat, 0},
	} {
		n := php8512NumericString(tc.s, tc.width, true)
		if n.kind != tc.kind || !n.trailing || (tc.kind == numericInt && n.integer != tc.i) {
			t.Errorf("%q width %d: %+v", tc.s, tc.width, n)
		}
	}
}
