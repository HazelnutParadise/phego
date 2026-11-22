package phego

import (
	"context"
	"testing"
)

// W-17 stage 7B: REQ-026. The closure hot paths, run from PHP source
// through the public engine, next to the plain function call they are
// measured against. Each program does 100,000 iterations.
var w17Stage7BWorkloads = []struct{ name, src string }{
	{"function_call", `<?php
function inc($n) { return $n + 1; }
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = inc($x); }
return $x;`},
	{"function_call_4_arguments", `<?php
function inc($n, $a, $b, $c) { return $n + $a + $b + $c; }
$x = 0; $a = 1; $b = 0; $c = 0;
for ($i = 0; $i < 100000; $i++) { $x = inc($x, $a, $b, $c); }
return $x;`},
	{"closure_call", `<?php
$inc = function ($n) { return $n + 1; };
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $inc($x); }
return $x;`},
	{"closure_call_3_captures", `<?php
$a = 1; $b = 0; $c = 0;
$inc = function ($n) use ($a, $b, $c) { return $n + $a + $b + $c; };
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $inc($x); }
return $x;`},
	{"closure_call_capture_by_reference", `<?php
$x = 0;
$inc = function () use (&$x) { $x = $x + 1; };
for ($i = 0; $i < 100000; $i++) { $inc(); }
return $x;`},
	{"arrow_call", `<?php
$a = 1;
$inc = fn($n) => $n + $a;
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $inc($x); }
return $x;`},
	{"bound_closure_call", `<?php
class C { private $n = 0; public function bumper() { return function () { return ++$this->n; }; } }
$bump = (new C)->bumper(); $x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $bump(); }
return $x;`},
	{"first_class_callable_call", `<?php
function inc($n) { return $n + 1; }
$inc = inc(...);
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $inc($x); }
return $x;`},
	{"closure_static_variable", `<?php
$next = function () { static $n = 0; return ++$n; };
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $next(); }
return $x;`},
	{"create_closure", `<?php
$x = 0;
for ($i = 0; $i < 100000; $i++) { $f = function ($n) { return $n; }; $x = $x + 1; }
return $x;`},
	{"create_closure_3_captures", `<?php
$a = 1; $b = 2; $c = 3; $x = 0;
for ($i = 0; $i < 100000; $i++) { $f = function ($n) use ($a, $b, $c) { return $n; }; $x = $x + 1; }
return $x;`},
	{"create_first_class_callable", `<?php
function inc($n) { return $n + 1; }
$x = 0;
for ($i = 0; $i < 100000; $i++) { $f = inc(...); $x = $x + 1; }
return $x;`},
}

func BenchmarkW17Stage7B(b *testing.B) {
	e, err := NewEngine(Config{})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for _, w := range w17Stage7BWorkloads {
		p, err := e.Compile(ctx, "/w/bench.php", w.src)
		if err != nil {
			b.Fatalf("%s: %v", w.name, err)
		}
		b.Run(w.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				res, err := e.Run(ctx, p)
				if err != nil {
					b.Fatal(err)
				}
				if n, ok := res.Return.Int64(); !ok || n != 100000 {
					b.Fatalf("returned %v", res.Return)
				}
			}
		})
	}
}
