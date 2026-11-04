package ir

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCompatibilityNumericLimits(t *testing.T) {
	cases := []struct{ name, expr, want string }{
		{"large_strict", `"` + strings.Repeat("a", 5000) + `" === "` + strings.Repeat("a", 5000) + `"`, "true"},
		{"large_concat_truth", `!("` + strings.Repeat("a", 4097) + `" . "")`, "false"},
		{"nan_less_number", `(1e999-1e999) < 0`, "false"},
		{"nan_greater_number", `(1e999-1e999) > 0`, "false"},
		{"nan_less_string", `(1e999-1e999) < "x"`, "false"},
		{"nan_greater_string", `(1e999-1e999) > "A"`, "false"},
		{"nan_array", `[(1e999-1e999)] == [(1e999-1e999)]`, "false"},
		{"shortcircuit", `false && [[]=>1]`, "false"},
		{"long_key", `["` + strings.Repeat("a", 5000) + `" => 1]`, "array"},
	}
	for _, width := range []int{32, 64} {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := lowerSource("<?php ("+tc.expr+")::class;", width)
				var ce *CompileError
				want := `Cannot use "::class" on ` + tc.want
				if !errors.As(err, &ce) || ce.Message != want {
					t.Errorf("width=%d got %v want %q", width, err, want)
				}
			})
		}
	}
}

// Resource exhaustion is a host limit, distinct from PHP's CompileError and
// from a dynamic expression whose evaluation is deferred until execution.
func TestCompatibilityConstantResourceLimit(t *testing.T) {
	src := "<?php (!(" + strings.TrimSuffix(strings.Repeat(`"`+strings.Repeat("a", 4096)+`" . `, 128), " . ") + "))::class;"
	_, err := lowerSource(src, 64)
	var phpError *CompileError
	if err == nil || errors.As(err, &phpError) || !strings.Contains(err.Error(), "constant evaluation resource limit") {
		t.Fatalf("expected distinct host limit, got %T: %v", err, err)
	}
	// The unvisited branch must not spend its evaluation budget.
	_, err = lowerSource(strings.Replace(src, "(!(", "(false && !(", 1), 64)
	if !errors.As(err, &phpError) || phpError.Message != `Cannot use "::class" on false` {
		t.Fatalf("short circuit: %v", err)
	}
}

func TestCompatibilityConstantDepthAndUnionLimits(t *testing.T) {
	parts := make([]string, 512)
	for i := range parts {
		parts[i] = fmt.Sprintf("['k%d'=>%d]", i, i)
	}
	for name, expr := range map[string]string{
		"deep":  strings.Repeat("!", 2050) + "1",
		"union": strings.Join(parts, "+"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := lowerSource("<?php ("+expr+")::class;", 64)
			var limit *constantLimitError
			if !errors.As(err, &limit) {
				t.Fatalf("expected host resource limit, got %T: %v", err, err)
			}
		})
	}
}
