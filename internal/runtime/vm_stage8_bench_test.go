package runtime

import (
	"context"
	"testing"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026. The hot paths of
// generators: foreach resuming a generator (each step is a resume and a
// yield), and creating many small generators (the frame moves into the
// generator's storage). The two storage layouts of a generator's frame run
// in the same binary.

const vmGenForeachSrc = `<?php
function gen($n) { for ($i = 0; $i < $n; $i++) { yield $i => $i; } }
$s = 0;
foreach (gen(2000) as $k => $v) { $s += $v; }
return $s;
`

const vmGenCreateSrc = `<?php
function gen($a, $b) { $c = $a + $b; yield $c; }
$s = 0;
for ($i = 0; $i < 500; $i++) { $g = gen($i, 1); $s += $g->current(); }
return $s;
`

func BenchmarkVMGenerator(b *testing.B) {
	for _, w := range []struct{ name, src string }{{"foreach", vmGenForeachSrc}, {"create", vmGenCreateSrc}} {
		p := phpProgram(b, w.src)
		for _, layout := range []struct {
			name  string
			split bool
		}{{"oneblock", false}, {"split", true}} {
			b.Run(w.name+"/layout="+layout.name, func(b *testing.B) {
				saved := vmGenSplitStorage
				vmGenSplitStorage = layout.split
				defer func() { vmGenSplitStorage = saved }()
				cfg := vmConfig(b, 64)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cfg.Context = ctx
				res := p.Run(cfg)
				if res.End != Completed {
					b.Fatalf("%v %v", res.End, res.Err)
				}
				req := make([]*Request, b.N)
				for i := range req {
					req[i] = readyRequest(b)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cfg.Request = req[i]
					p.Run(cfg)
				}
				b.ReportMetric(float64(res.Steps), "steps/op")
			})
		}
	}
}
