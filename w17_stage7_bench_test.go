package phego

import (
	"context"
	"testing"
)

// W-17 stage 7A: REQ-026. The object hot paths, run from PHP source through
// the public engine: method calls, property reads and writes, static calls
// and object creation, next to a plain function call for scale. Each
// program does 100,000 iterations.
var w17Stage7Workloads = []struct{ name, src string }{
	{"function_call", `<?php
function inc($n) { return $n + 1; }
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = inc($x); }
return $x;`},
	{"method_call", `<?php
class C { public function inc($n) { return $n + 1; } }
$o = new C; $x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $o->inc($x); }
return $x;`},
	{"static_call", `<?php
class C { public static function inc($n) { return $n + 1; } }
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = C::inc($x); }
return $x;`},
	{"property_read_write", `<?php
class C { public $n = 0; }
$o = new C;
for ($i = 0; $i < 100000; $i++) { $o->n = $o->n + 1; }
return $o->n;`},
	{"this_property_in_method", `<?php
class C { private $n = 0; public function bump() { $this->n = $this->n + 1; return $this->n; } }
$o = new C; $x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $o->bump(); }
return $x;`},
	{"typed_method", `<?php
class C { public function inc(int $n): int { return $n + 1; } }
$o = new C; $x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $o->inc($x); }
return $x;`},
	{"new_object", `<?php
class P { public $x; public $y; public function __construct($x, $y) { $this->x = $x; $this->y = $y; } }
$s = 0;
for ($i = 0; $i < 100000; $i++) { $p = new P($i, 1); $s = $s + $p->y; }
return $s;`},
}

func BenchmarkW17Stage7(b *testing.B) {
	e, err := NewEngine(Config{})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for _, w := range w17Stage7Workloads {
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
