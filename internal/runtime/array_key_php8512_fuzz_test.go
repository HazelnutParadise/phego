package runtime

import (
	"strconv"
	"testing"
)

// FuzzPHP8512IntegerStringKey checks the decimal spelling and width bounds
// independently of the implementation's digit-by-digit arithmetic.
func FuzzPHP8512IntegerStringKey(f *testing.F) {
	for _, s := range []string{"", "0", "-0", "01", "-1", "+1", "1\x00", "2147483648", "-2147483648", "9223372036854775807", "-9223372036854775808", "9223372036854775808", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, width := range []int{32, 64} {
			want, parseErr := strconv.ParseInt(s, 10, width)
			valid := parseErr == nil && strconv.FormatInt(want, 10) == s
			got, ok, err := php8512IntegerStringKey(s, width)
			if err != nil || ok != valid || (ok && got != want) {
				t.Fatalf("key %q width %d: got (%d, %t, %v), want (%d, %t, nil)", s, width, got, ok, err, want, valid)
			}
		}
	})
}
