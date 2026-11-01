package compat

import (
	"os"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// FuzzScan checks that scanning any program the parser accepts never panics.
func FuzzScan(f *testing.F) {
	if src, err := os.ReadFile("testdata/site.php"); err == nil {
		f.Add(string(src))
	}
	for _, s := range []string{`<?php array_udiff();`, `<?php f(...$a);`, `<?php use function A\{b as c}; c();`,
		`<?php include("x" . $y);`, `<?php call_user_func("\u{73}ystem");`, `<?php new ReflectionFunction();`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: 64, MaxBytes: 1 << 16})
		if err != nil {
			return
		}
		for _, finding := range Scan("f.php", tree) {
			if finding.Impact == "" || finding.Kind == "" {
				t.Fatalf("incomplete finding %+v", finding)
			}
		}
	})
}
