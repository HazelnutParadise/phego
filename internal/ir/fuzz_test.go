package ir

import (
	"errors"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// FuzzLower checks that any source the front end accepts lowers to a unit
// or a located compile error, without panicking.
func FuzzLower(f *testing.F) {
	for _, s := range []string{
		"<?php $a = fn($x) => fn($y) => $x + $y + $z;",
		"<?php namespace A { use B\\{C, function d}; function e(int|C ...$f): ?static {} }",
		"<?php class K { function m() { return parent::$p::get(); } }",
		"<?php [$a, [, 'k' => &$b]] = $c; list(,$d) = $e;",
		"<?php try { goto a; } finally { a: break; }",
		"<?php $s = <<<EOT\n  a $b {$c[1]} ${d}\n  EOT;",
		"<?php declare(strict_types=1); const X = A::B ?? new C(...[]);",
	} {
		f.Add(s)
	}
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, src string) {
		tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: 64, MaxBytes: 1 << 16})
		if err != nil {
			return
		}
		u, err := Lower(tree, prof, Options{File: "/fuzz.php"})
		var ce *CompileError
		switch {
		case err != nil && !errors.As(err, &ce):
			t.Fatalf("unexpected error type %T: %v", err, err)
		case err != nil && ce.Line < 1:
			t.Fatalf("compile error without a line: %v", err)
		case err == nil && u.Main == nil:
			t.Fatal("unit without main code")
		}
	})
}
