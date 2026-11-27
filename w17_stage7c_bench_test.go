package phego

import (
	"context"
	"testing"
)

// W-17 stage 7C: REQ-026. A try block costs nothing while no exception is
// thrown: the same loop inside and outside try, a loop whose body is a
// try/catch or a try/finally, and, for scale, a loop that throws and
// catches each time. Each program does 100,000 iterations.
var w17Stage7CWorkloads = []struct{ name, src string }{
	{"loop_plain", `<?php
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $x + $i % 7; }
return $x;`},
	{"loop_inside_try", `<?php
$x = 0;
try {
    for ($i = 0; $i < 100000; $i++) { $x = $x + $i % 7; }
} catch (Exception $e) { $x = -1; }
return $x;`},
	{"try_catch_in_loop_body", `<?php
$x = 0;
for ($i = 0; $i < 100000; $i++) { try { $x = $x + $i % 7; } catch (Exception $e) { $x = -1; } }
return $x;`},
	// The statements of the try/finally workload below without the try: its
	// finally block does one more assignment than the plain loop.
	{"loop_two_statements", `<?php
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = $x + $i % 7; $x = $x + 0; }
return $x;`},
	{"try_finally_in_loop_body", `<?php
$x = 0;
for ($i = 0; $i < 100000; $i++) { try { $x = $x + $i % 7; } finally { $x = $x + 0; } }
return $x;`},
	{"call_plain", `<?php
function inc($n) { return $n + 1; }
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = inc($x); }
return $x;`},
	{"call_with_try_in_callee", `<?php
function inc($n) { try { return $n + 1; } catch (Exception $e) { return 0; } }
$x = 0;
for ($i = 0; $i < 100000; $i++) { $x = inc($x); }
return $x;`},
	{"throw_and_catch", `<?php
$x = 0;
for ($i = 0; $i < 100000; $i++) { try { throw new Exception("e"); } catch (Exception $e) { $x = $x + 1; } }
return $x;`},
}

func BenchmarkW17Stage7C(b *testing.B) {
	e, err := NewEngine(Config{})
	if err != nil {
		b.Fatal(err)
	}
	for _, w := range w17Stage7CWorkloads {
		p, err := e.Compile(context.Background(), "/w/bench.php", w.src)
		if err != nil {
			b.Fatal(w.name, err)
		}
		b.Run(w.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r, err := e.Run(context.Background(), p)
				if err != nil || r.Status != StatusCompleted {
					b.Fatal(r, err)
				}
			}
		})
	}
}
