package runtime

import "testing"

func TestPHP8512IntegerStringKey(t *testing.T) {
	cases := []struct {
		key     string
		width   int
		want    int64
		numeric bool
	}{
		{"0", 64, 0, true}, {"42", 64, 42, true}, {"-42", 64, -42, true},
		{"", 64, 0, false}, {"-", 64, 0, false}, {"-0", 64, 0, false},
		{"00", 64, 0, false}, {"01", 64, 0, false}, {"-01", 64, 0, false},
		{"+1", 64, 0, false}, {" 1", 64, 0, false}, {"1 ", 64, 0, false},
		{"1.0", 64, 0, false}, {"1e2", 64, 0, false}, {"1\x00", 64, 0, false},
		{"2147483647", 32, 2147483647, true}, {"-2147483648", 32, -2147483648, true},
		{"2147483648", 32, 0, false}, {"-2147483649", 32, 0, false},
		{"9223372036854775807", 64, 9223372036854775807, true},
		{"-9223372036854775808", 64, -9223372036854775808, true},
		{"9223372036854775808", 64, 0, false}, {"-9223372036854775809", 64, 0, false},
	}
	for _, c := range cases {
		got, ok, err := php8512IntegerStringKey(c.key, c.width)
		if err != nil || ok != c.numeric || (ok && got != c.want) {
			t.Errorf("key %q width %d: got (%d, %t, %v), want (%d, %t, nil)", c.key, c.width, got, ok, err, c.want, c.numeric)
		}
	}
}

func TestPHP8512IntegerStringKeyRejectsInvalidWidth(t *testing.T) {
	for _, width := range []int{0, 16, 128} {
		if _, _, err := php8512IntegerStringKey("1", width); err == nil {
			t.Errorf("width %d: expected error", width)
		}
	}
}

func TestPHP8512IntegerStringKeyOtherBytes(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, key := range []string{"１２", "١", "\xff", "1\xff", "-00", "99999999999999999999999999999999999999999999999999"} {
			if n, ok, err := php8512IntegerStringKey(key, width); err != nil || ok {
				t.Errorf("key %q width %d: got %d, %t, %v", key, width, n, ok, err)
			}
		}
	}
	n, ok, err := php8512IntegerStringKey("2147483648", 64)
	if err != nil || !ok || n != 2147483648 {
		t.Fatalf("64-bit key: got %d, %t, %v", n, ok, err)
	}
}
