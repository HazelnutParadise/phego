package compat

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

func scanFile(t *testing.T, path string) []Finding {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := frontend.Parse(src, frontend.LexOptions{IntWidth: 64, MaxBytes: len(src)})
	if err != nil {
		t.Fatal(err)
	}
	return Scan(path, tree)
}

// TestScanReportsLocatedFindings checks every finding of a fixture site,
// including that statically known calls produce no finding.
func TestScanReportsLocatedFindings(t *testing.T) {
	var got []string
	for _, f := range scanFile(t, "testdata/site.php") {
		if f.File != "testdata/site.php" || f.Impact == "" {
			t.Errorf("incomplete finding %+v", f)
		}
		got = append(got, fmt.Sprintf("%d %s %s %s %s", f.Line, f.Kind, f.Class, f.Capability, f.Evidence))
	}
	want := []string{
		"2 include capability filesystem.read require __DIR__.'/config.php'",
		"3 include capability filesystem.read include 'lib/util.php'",
		"4 dynamic-include unknown filesystem.read include binary expression",
		"7 dynamic-call unknown  $handler",
		"9 dynamic-method unknown  prop expression",
		"11 dynamic-static-call unknown  class_const expression",
		"13 dynamic-new unknown  $widgetClass",
		"15 dynamic-callback unknown  call_user_func($callback)",
		"18 eval unknown  eval($code)",
		"19 variable-variable unknown  $$name",
		"20 shell capability process `...`",
		"21 process capability process shell_exec()",
		"22 file-write capability filesystem.write file_put_contents('/var/www/uploads/a.txt')",
		"23 file-write unknown filesystem.write file_put_contents($target)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestScanNeverDropsDynamicTargets builds call shapes whose target is only
// known at run time and requires each to be reported as Unknown.
func TestScanNeverDropsDynamicTargets(t *testing.T) {
	for _, src := range []string{
		`<?php ('str' . 'tolower')('X');`,
		`<?php $a['f']();`,
		`<?php ($f)();`,
		`<?php include_once $p;`,
		`<?php require dirname(__FILE__) . '/x.php';`,
		`<?php call_user_func_array(...$args);`,
		`<?php $o->{$m}();`,
		`<?php new ($c . 'X');`,
	} {
		tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: 64, MaxBytes: len(src)})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		fs := Scan("t.php", tree)
		if len(fs) == 0 || fs[0].Class != Unknown {
			t.Errorf("%s: findings %+v", src, fs)
		}
	}
}
