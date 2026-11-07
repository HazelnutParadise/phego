package ir

import (
	"errors"
	"testing"
)

// Expectations were checked with the pinned PHP 8.5.12 CLI using php -n -l.
func TestCompatibilityConstantExpressions(t *testing.T) {
	for _, tc := range []struct {
		source string
		error  string
	}{
		{`<?php (1+1)::class;`, `Cannot use "::class" on int`},
		{`<?php (1.0+1)::class;`, `Cannot use "::class" on float`},
		{`<?php (1-1)::class;`, `Cannot use "::class" on int`},
		{`<?php (1*2)::class;`, `Cannot use "::class" on int`},
		{`<?php (4/2)::class;`, `Cannot use "::class" on int`},
		{`<?php (3/2)::class;`, `Cannot use "::class" on float`},
		{`<?php (3%2)::class;`, `Cannot use "::class" on int`},
		{`<?php (2**3)::class;`, `Cannot use "::class" on int`},
		{`<?php (1<<1)::class;`, `Cannot use "::class" on int`},
		{`<?php (1>>1)::class;`, `Cannot use "::class" on int`},
		{`<?php (1&2)::class;`, `Cannot use "::class" on int`},
		{`<?php (1|2)::class;`, `Cannot use "::class" on int`},
		{`<?php (1^2)::class;`, `Cannot use "::class" on int`},
		{`<?php (1==1)::class;`, `Cannot use "::class" on true`},
		{`<?php (1===1)::class;`, `Cannot use "::class" on true`},
		{`<?php (!false)::class;`, `Cannot use "::class" on true`},
		{`<?php (~1)::class;`, `Cannot use "::class" on int`},
		{`<?php ((1+1)+2)::class;`, `Cannot use "::class" on int`},
		{`<?php (1+"2")::class;`, `Cannot use "::class" on int`},
		{`<?php ([]+[])::class;`, `Cannot use "::class" on array`},
		{`<?php ("A")::class;`, ""},
		{`<?php ("A"."B")::class;`, ""},
		{`<?php (1/0)::class;`, ""},
		{`<?php (1+"a")::class;`, ""},
		{`<?php (true ? 1 : 2)::class;`, ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			_, err := lowerSource(tc.source, 64)
			if tc.error == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var ce *CompileError
			if !errors.As(err, &ce) || ce.Message != tc.error || ce.Line != 1 {
				t.Fatalf("got %v, want compile error on line 1: %s", err, tc.error)
			}
		})
	}
}

func TestCompatibilityConstantPropertyName(t *testing.T) {
	for _, tc := range []struct {
		source string
		warn   bool
	}{
		{`<?php $o->{[]};`, true},
		{`<?php $o->{[1,2]};`, true},
		{`<?php $o->{[foo()]};`, false},
		{`<?php $o->{"A"."B"};`, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			u := lower(t, tc.source)
			if !tc.warn {
				if len(u.Diagnostics) != 0 {
					t.Fatalf("unexpected diagnostics: %+v", u.Diagnostics)
				}
				return
			}
			want := Diagnostic{Line: 1, Level: "Warning", Message: "Array to string conversion"}
			if len(u.Diagnostics) != 1 || u.Diagnostics[0] != want {
				t.Fatalf("got %+v, want %+v", u.Diagnostics, want)
			}
		})
	}
}

func TestCompatibilityEmptyStatementBeforeDeclare(t *testing.T) {
	for _, tc := range []struct {
		source string
		bad    bool
	}{
		{`<?php ; declare(encoding="UTF-8");`, true},
		{`<?php ?><?php declare(encoding="UTF-8");`, true},
		{`<?php ; declare(ticks=1); declare(encoding="UTF-8");`, true},
		{`<?php declare(ticks=1); declare(encoding="UTF-8");`, false},
		{`<?php declare(strict_types=1); declare(encoding="UTF-8");`, false},
		{`<?php ; declare(strict_types=1);`, false},
		{`<?php ; namespace A;`, false},
		{`<?php namespace A {} ;`, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			_, err := lowerSource(tc.source, 64)
			if !tc.bad {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var ce *CompileError
			if !errors.As(err, &ce) || ce.Message != "Encoding declaration pragma must be the very first statement in the script" || ce.Line != 1 {
				t.Fatalf("expected encoding declaration error on line 1, got %v", err)
			}
		})
	}
}

func TestCompatibilityKnownFunctionArguments(t *testing.T) {
	for _, tc := range []struct {
		source string
		bad    bool
	}{
		{`<?php function f($x){} f($a[]);`, true},
		{`<?php function f($x){} f(x:$a[]);`, true},
		{`<?php function f(){} f($a[]);`, true},
		{`<?php function f(...$x){} f($a[]);`, true},
		{`<?php f($a[]); function f($x){}`, false},
		{`<?php function f(&$x){} f($a[]);`, false},
		{`<?php function f(&...$x){} f($a[]);`, false},
		{`<?php function f($x){} f(y:$a[]);`, false},
		{`<?php function f($x){} f(...$a,x:$b[]);`, false},
		{`<?php if (true) {function f($x){}} f($a[]);`, false},
		{`<?php function f($x){f($a[]);}`, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			_, err := lowerSource(tc.source, 64)
			if !tc.bad {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var ce *CompileError
			if !errors.As(err, &ce) || ce.Message != "Cannot use [] for reading" || ce.Line != 1 {
				t.Fatalf("expected array read compile error on line 1, got %v", err)
			}
		})
	}
}
