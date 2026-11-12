package phpname

import "testing"

func TestLowerFoldsOnlyASCII(t *testing.T) {
	for in, want := range map[string]string{"": "", "abc": "abc", "StrLen": "strlen", "NS\\F": "ns\\f", "É": "É", "aÉB": "aÉb", "\u212a": "\u212a"} {
		if got := Lower(in); got != want {
			t.Errorf("Lower(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEqualFoldFoldsOnlyASCII(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"", "", true}, {"Abc", "aBC", true}, {"abc", "abd", false}, {"ab", "abc", false}, {"\u00c9", "\u00e9", false}, {"k", "\u212a", false}, {"\u00e9X", "\u00e9x", true}} {
		if got := EqualFold(c.a, c.b); got != c.want {
			t.Errorf("EqualFold(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
