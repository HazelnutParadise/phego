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

// TestScanResolvesNamesAndCallbacks covers aliases, callback strings, and the
// functions that need a capability through any call form.
func TestScanResolvesNamesAndCallbacks(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string // "kind class capability" of the first finding, or "" for none
	}{
		{`<?php namespace A; use function shell_exec as sh; sh('id');`, "process capability process"},
		{`<?php use function call_user_func as cuf; cuf($x);`, "dynamic-callback unknown "},
		{`<?php use function B\{file_put_contents as fpc}; fpc($p, 'x');`, ""},
		{`<?php use function file_put_contents as fpc; fpc($p, 'x');`, "file-write unknown filesystem.write"},
		{`<?php call_user_func('system', 'id');`, "process capability process"},
		{`<?php array_map('shell_exec', $c);`, "process capability process"},
		{`<?php call_user_func('file_put_contents', $p, 'x');`, "file-write unknown filesystem.write"},
		{`<?php fopen($p, 'w');`, "file-write unknown filesystem.write"},
		{`<?php fopen('/tmp/x', 'a+');`, "file-write capability filesystem.write"},
		{`<?php fopen($p, 'r');`, ""},
		{`<?php fopen($p, $mode);`, "file-write unknown filesystem.write"},
		{`<?php symlink($a, $b);`, "file-write unknown filesystem.write"},
		{`<?php ob_start($cb);`, "dynamic-callback unknown "},
		{`<?php array_udiff($a, $b, $cb);`, "dynamic-callback unknown "},
		{`<?php preg_replace_callback_array($m, $s);`, "dynamic-callback unknown "},
		{`<?php Closure::fromCallable($x);`, "dynamic-callback unknown "},
		{`<?php extract($_GET);`, "extract unknown "},
		{`<?php mail($to, $s, $b);`, "process capability process"},
		{`<?php include('a.php');`, "include capability filesystem.read"},
		{`<?php require_once(__DIR__ . '/x.php');`, "include capability filesystem.read"},
		{`<?php include 'http://example.test/a.php';`, "remote-include capability network"},
		{`<?php array_filter($a);`, ""},
		{`<?php array_map(null, $a);`, ""},
		{`<?php \App\exec('x');`, ""},
		{`<?php namespace App; exec('x');`, "process capability process"},
		{`<?php $f = strlen(...);`, ""},
		{`<?php $f = system(...);`, "process capability process"},
		{`<?php array_udiff();`, ""},
		{`<?php array_filter(...[['echo C'], 'system']);`, "dynamic-callback unknown "},
		{`<?php usort(callback: 'system', array: $a);`, "dynamic-callback unknown "},
		{`<?php call_user_func('\\system', 'x');`, "process capability process"},
		{`<?php call_user_func("\x73ystem", 'x');`, "process capability process"},
		{`<?php fopen('/tmp/x', "\167");`, "file-write capability filesystem.write"},
		{`<?php (new ReflectionFunction('system'))->invoke('x');`, "process capability process"},
		{`<?php new \ReflectionFunction($f);`, "dynamic-callback unknown "},
		{`<?php Closure::fromCallable('system');`, "process capability process"},
		{`<?php call_user_func('extract', $a);`, "extract unknown "},
		{`<?php namespace\system('x');`, "process capability process"},
		{`<?php namespace A; namespace\system('x');`, ""},
		{`<?php array_find(['x'], 'system');`, "process capability process"},
		{`<?php filter_var($x, FILTER_CALLBACK, ['options' => 'system']);`, "dynamic-callback unknown "},
		{`<?php mb_send_mail($to, $s, $b);`, "process capability process"},
		{`<?php error_log($m, 3, '/var/log/app.log');`, "file-write capability filesystem.write"},
		{`<?php error_log($m, $type, $dest);`, "error-log unknown filesystem.write"},
		{`<?php error_log("m", '3', '/tmp/x');`, "file-write capability filesystem.write"},
		{`<?php error_log("m", 0x3, $p);`, "file-write unknown filesystem.write"},
		{`<?php error_log("m", 3.0, '/tmp/x');`, "file-write capability filesystem.write"},
		{`<?php error_log("m", ...$r);`, "error-log unknown filesystem.write"},
		{`<?php error_log(...$a);`, "error-log unknown filesystem.write"},
		{`<?php error_log("m", 1, "a@b.c");`, "process capability process"},
		{`<?php error_log("m", 0);`, ""},
		{`<?php call_user_func("e\X78ec", "id");`, "process capability process"},
		{`<?php fopen('/tmp/f', "\X77");`, "file-write capability filesystem.write"},
		{`<?php filter_var($v, $f, $o);`, "dynamic-callback unknown "},
		{`<?php filter_var($v, 1024, ['options' => 'exec']);`, "dynamic-callback unknown "},
		{`<?php filter_var($v, FILTER_VALIDATE_INT);`, ""},
		{`<?php filter_var($v);`, ""},
		{`<?php filter_var_array($a, ['x' => FILTER_VALIDATE_INT]);`, ""},
		{`<?php filter_var_array($a, ['x' => ['filter' => 1024]]);`, "dynamic-callback unknown "},
		{`<?php filter_var_array($a, $def);`, "dynamic-callback unknown "},
		{`<?php use ReflectionFunction as RF; (new RF('exec'))->invoke('x');`, "process capability process"},
		{`<?php use Closure as C; C::fromCallable('exec');`, "process capability process"},
		{`<?php namespace A; new ReflectionFunction('exec');`, ""},
		{`<?php include 'data:text/plain,<?php echo 1;';`, "stream-include unknown stream"},
		{`<?php filter_var_array($a, ['x' => ['filter' => '1024', 'options' => 'system']]);`, "dynamic-callback unknown "},
		{`<?php filter_var_array($a, ['x' => ['filter' => 1000 + 24]]);`, "dynamic-callback unknown "},
		{`<?php filter_var_array($a, ['x' => ['filter' => Foo::F]]);`, "dynamic-callback unknown "},
		{`<?php filter_var_array($a, ['x' => self::RULE]);`, "dynamic-callback unknown "},
		{`<?php filter_var_array($a, ['x' => ['filter' => FILTER_VALIDATE_INT, 'flags' => FILTER_NULL_ON_FAILURE]]);`, ""},
		{`<?php call_user_func('filter_var', 'id', FILTER_CALLBACK, ['options' => 'system']);`, "dynamic-callback unknown "},
		{`<?php new namespace\ReflectionFunction('exec');`, "process capability process"},
		{`<?php namespace\Closure::fromCallable('exec');`, "process capability process"},
		{`<?php error_log($m);`, ""},
		{`<?php include 'php://input';`, "stream-include unknown stream"},
		{`<?php include 'data://text/plain,<?php echo 1;';`, "stream-include unknown stream"},
	} {
		tree, err := frontend.Parse([]byte(tc.src), frontend.LexOptions{IntWidth: 64, MaxBytes: len(tc.src)})
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		got := ""
		if fs := Scan("t.php", tree); len(fs) > 0 {
			got = fmt.Sprintf("%s %s %s", fs[0].Kind, fs[0].Class, fs[0].Capability)
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}
