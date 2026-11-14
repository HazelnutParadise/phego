package phpconst

import (
	"math"
	"sort"
	"testing"
)

// REQ-016, REQ-025: the predefined constant tables of PHP 8.5.12.

func TestTablesAreSortedByKey(t *testing.T) {
	for width, table := range map[int][]Constant{64: table64, 32: table32} {
		if !sort.SliceIsSorted(table, func(i, j int) bool { return table[i].Name < table[j].Name }) {
			t.Fatalf("the %d-bit table is not sorted by key", width)
		}
		for i := 1; i < len(table); i++ {
			if table[i].Name == table[i-1].Name {
				t.Fatalf("the %d-bit table repeats %q", width, table[i].Name)
			}
		}
		if len(table) != 1792 {
			t.Fatalf("the %d-bit table holds %d constants, the images define 1792", width, len(table))
		}
		if Count(width) != len(table) {
			t.Fatalf("Count(%d) = %d", width, Count(width))
		}
	}
}

func TestKeyAndShort(t *testing.T) {
	for _, tc := range []struct{ name, key, short string }{
		{"PHP_EOL", "PHP_EOL", "PHP_EOL"},
		{"Dom\\INDEX_SIZE_ERR", "dom\\INDEX_SIZE_ERR", "INDEX_SIZE_ERR"},
		{"A\\B\\c", "a\\b\\c", "c"},
		{"ns\\X", "ns\\X", "X"},
	} {
		if got := Key(tc.name); got != tc.key {
			t.Errorf("Key(%q) = %q, want %q", tc.name, got, tc.key)
		}
		if got := Short(tc.name); got != tc.short {
			t.Errorf("Short(%q) = %q, want %q", tc.name, got, tc.short)
		}
	}
}

// Values the pinned images printed for the two widths (reference-*.json
// and the probes of 2026-11-14).
func TestLookupValues(t *testing.T) {
	type want struct {
		kind Kind
		i    int64
		f    float64
		s    string
	}
	cases := map[int]map[string]want{
		64: {
			"PHP_INT_SIZE": {Int, 8, 0, ""}, "PHP_INT_MAX": {Int, math.MaxInt64, 0, ""}, "PHP_INT_MIN": {Int, math.MinInt64, 0, ""},
			"CREDITS_ALL": {Int, 4294967295, 0, ""},
		},
		32: {
			"PHP_INT_SIZE": {Int, 4, 0, ""}, "PHP_INT_MAX": {Int, math.MaxInt32, 0, ""}, "PHP_INT_MIN": {Int, math.MinInt32, 0, ""},
			"CREDITS_ALL": {Int, -1, 0, ""},
		},
	}
	common := map[string]want{
		"PHP_EOL": {String, 0, 0, "\n"}, "PHP_OS": {String, 0, 0, "Linux"}, "PHP_OS_FAMILY": {String, 0, 0, "Linux"},
		"DIRECTORY_SEPARATOR": {String, 0, 0, "/"}, "PHP_VERSION": {String, 0, 0, "8.5.12"},
		"E_ALL": {Int, 30719, 0, ""}, "E_WARNING": {Int, 2, 0, ""}, "E_STRICT": {Int, 2048, 0, ""},
		"M_PI": {Float, 0, math.Pi, ""}, "PHP_FLOAT_EPSILON": {Float, 0, 2.220446049250313e-16, ""},
		"INF":                 {Float, 0, math.Inf(1), ""},
		"dom\\INDEX_SIZE_ERR": {Int, 1, 0, ""},
		"STDIN":               {Resource, 0, 0, ""}, "CHAR_MAX": {Platform, 0, 0, ""}, "PHP_BUILD_DATE": {Platform, 0, 0, ""},
	}
	for width, table := range cases {
		for name, w := range common {
			table[name] = w
		}
		for name, w := range table {
			c, ok := Lookup(width, name)
			if !ok {
				t.Errorf("%d-bit: %s missing", width, name)
				continue
			}
			if c.Kind != w.kind || c.Int != w.i || c.Str != w.s || (c.Float != w.f && !(math.IsNaN(c.Float) && math.IsNaN(w.f))) {
				t.Errorf("%d-bit: %s = %+v, want %+v", width, name, c, w)
			}
		}
		if c, ok := Lookup(width, "NAN"); !ok || !math.IsNaN(c.Float) {
			t.Errorf("%d-bit: NAN = %+v, %v", width, c, ok)
		}
		if c, ok := Lookup(width, "E_STRICT"); !ok || c.Deprecated != "Constant E_STRICT is deprecated since 8.4, the error level was removed" {
			t.Errorf("%d-bit: E_STRICT deprecation = %q", width, c.Deprecated)
		}
		if c, ok := Lookup(width, "E_ALL"); !ok || c.Deprecated != "" {
			t.Errorf("%d-bit: E_ALL deprecation = %q", width, c.Deprecated)
		}
		for _, missing := range []string{"true", "null", "php_eol", "Dom\\INDEX_SIZE_ERR", "", "X"} {
			if _, ok := Lookup(width, missing); ok {
				t.Errorf("%d-bit: %q found", width, missing)
			}
		}
	}
}
