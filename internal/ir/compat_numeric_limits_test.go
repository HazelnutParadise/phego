package ir

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
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
// PHP 8.5.12 folds this 2,100-deep right-nested sum (php -n -l reports
// "Cannot use "::class" on false"); right nesting beyond the evaluation
// depth is a host limit here.
func TestCompatibilityConstantResourceLimit(t *testing.T) {
	src := "<?php (!(" + strings.Repeat("1 + (", 2100) + "1" + strings.Repeat(")", 2100) + "))::class;"
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

func arrayUnion(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("['k%d'=>%d]", i, i)
	}
	return strings.Join(parts, "+")
}

// A concatenation chain costs its result length, as in PHP, not the sum of
// every intermediate string. PHP 8.5.12 folds this 512 KiB chain.
func TestCompatibilityLongConcatenationFolds(t *testing.T) {
	src := "<?php (!(" + strings.TrimSuffix(strings.Repeat(`"`+strings.Repeat("a", 4096)+`" . `, 128), " . ") + "))::class;"
	_, err := lowerSource(src, 64)
	var phpError *CompileError
	if !errors.As(err, &phpError) || phpError.Message != `Cannot use "::class" on false` {
		t.Fatalf("got %T: %v", err, err)
	}
}

// A 2,050-deep unary chain folds iteratively, and a union chain costs its
// operands' total size; php -n -l reports these messages.
func TestCompatibilityConstantDeepChainAndUnionFold(t *testing.T) {
	for expr, want := range map[string]string{
		strings.Repeat("!", 2050) + "1": `Cannot use "::class" on true`,
		arrayUnion(512):                 `Cannot use "::class" on array`,
		"!(" + arrayUnion(6000) + ")":   `Cannot use "::class" on false`,
	} {
		_, err := lowerSource("<?php ("+expr+")::class;", 64)
		var phpError *CompileError
		if !errors.As(err, &phpError) || phpError.Message != want {
			t.Errorf("%.20s...: got %T: %v, want %q", expr, err, err, want)
		}
	}
}

// __DIR__ and __FILE__ are as long as the host path; an expression using them
// is not limited by its own source length.
func TestConstantMagicPathDrawsOnTheSharedReserve(t *testing.T) {
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	src := "<?php class A { public string $p = __DIR__ . \"/data\"; }\nconst B = __DIR__ . \"/x\";"
	tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: 64, MaxBytes: len(src) + 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lower(tree, prof, Options{File: "/" + strings.Repeat("d/", 1024) + "t.php"}); err != nil {
		t.Fatal(err)
	}
}
