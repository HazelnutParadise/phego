package runtime

import (
	"context"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// vmFuzzImage builds a random image the verifier accepts from seed: every
// register is defined in the prologue, so any instruction order and any jump
// target is valid by dataflow.
func vmFuzzImage(t testing.TB, seed uint64, width int) *vmAsm {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	a := newAsm(t, width)
	consts := []any{nil, false, true, 0, 1, -1, 7, int64(math.MaxInt32), 1.5, -0.0, math.Inf(1), math.NaN(), "", "0", "1", "abc", "1.5", " 5", "z", "9223372036854775807", "\x00"}
	body := func(f *vmFn, nfuncs int) {
		const nv = 8
		vals := make([]uint32, nv)
		for i := range vals {
			vals[i] = f.k(consts[rng.IntN(len(consts))])
		}
		names := []string{"a", "b", "c", "d"}
		v := func() uint32 { return vals[rng.IntN(nv)] }
		start := f.pc()
		n := 1 + rng.IntN(40)
		var jumps []int
		for i := 0; i < n; i++ {
			f.at(uint32(2 + rng.IntN(50)))
			dst := v()
			switch rng.IntN(24) {
			case 0:
				f.emit(bytecode.OpConst, 0, dst, f.a.constant(bytecode.Constant{Kind: bytecode.Int, Bits: uint64(rng.Int64N(100) - 50)}), 0)
			case 1:
				f.emit(bytecode.OpBinary, uint16(1+rng.IntN(20)), dst, v(), v())
			case 2:
				f.emit(bytecode.OpUnary, uint16(1+rng.IntN(4)), dst, v(), 0)
			case 3:
				f.emit(bytecode.OpCast, uint16(1+rng.IntN(5)), dst, v(), 0)
			case 4:
				f.emit(bytecode.OpLoad, uint16(rng.IntN(2)), dst, f.loc(names[rng.IntN(4)]), 0)
			case 5:
				f.store(names[rng.IntN(4)], v())
			case 6:
				f.emit(bytecode.OpIncDec, uint16(1+rng.IntN(4)), dst, f.loc(names[rng.IntN(4)]), 0)
			case 7:
				f.set(names[rng.IntN(4)], v(), v())
			case 8:
				f.push(names[rng.IntN(4)], v())
			case 9:
				f.unsetKey(names[rng.IntN(4)], v())
			case 10:
				f.emit(bytecode.OpArrayGet, uint16(rng.IntN(2)), dst, v(), v())
			case 11:
				f.emit(bytecode.OpArrayIsset, 0, dst, v(), v())
			case 12:
				f.emit(bytecode.OpIsset, 0, dst, f.loc(names[rng.IntN(4)]), 0)
			case 13:
				f.unset(names[rng.IntN(4)])
			case 14:
				f.emit(bytecode.OpNewArray, 0, dst, 0, 0)
			case 15:
				f.echo(v())
			case 16:
				jumps = append(jumps, f.emit(bytecode.OpBranch, 0, v(), 0, 0))
			case 17:
				jumps = append(jumps, f.emit(bytecode.OpJump, 0, 0, 0, 0))
			case 18:
				fi := rng.IntN(nfuncs)
				argc := rng.IntN(3)
				for j := 0; j < argc; j++ {
					f.emit(bytecode.OpPush, 0, v(), 0, 0)
				}
				if rng.IntN(2) == 0 && argc == 1 && fi > 0 {
					f.emit(bytecode.OpCall, 0, dst, uint32(fi), uint32(argc))
				} else {
					name := []string{"F1", "f2", "missing", "{main}"}[rng.IntN(4)]
					f.emit(bytecode.OpConst, 0, dst, f.a.str(name), 0)
					f.emit(bytecode.OpCallValue, 0, v(), dst, uint32(argc))
				}
			case 20:
				if arr := v(); rng.IntN(2) == 0 {
					f.emit(bytecode.OpArrayAdd, 0, arr, v(), 0)
				} else {
					f.add(arr, v(), v())
				}
			case 21:
				f.addVarKey(v(), names[rng.IntN(4)], v())
			case 22:
				key, val := "", ""
				if rng.IntN(2) == 0 {
					key = names[rng.IntN(4)]
				}
				if rng.IntN(2) == 0 {
					val = names[rng.IntN(4)]
				}
				f.setVars(names[rng.IntN(4)], key, v(), val, v())
			case 23:
				f.pushVar(names[rng.IntN(4)], names[rng.IntN(4)])
			case 19:
				if rng.IntN(4) == 0 {
					f.exitWith(v())
				} else {
					f.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
				}
			}
		}
		end := f.pc()
		f.ret(v())
		for _, pc := range jumps {
			target := start + uint32(rng.IntN(int(end-start)+1))
			other := start + uint32(rng.IntN(int(end-start)+1))
			f.patch(pc, target, other)
		}
	}
	main := a.fn("{main}", 0, 2, "a", "b", "c", "d")
	f1 := a.fn("F1", 1, 30, "a", "b", "c", "d")
	f2 := a.fn("f2", 1, 40, "a", "b", "c", "d")
	body(main, 3)
	body(f1, 3)
	body(f2, 3)
	return a
}

var vmRunEnds = map[RunEnd]bool{Completed: true, Exited: true, UncaughtThrowable: true, Fatal: true, Cancelled: true, LimitExceeded: true, OutputFailed: true, Unsupported: true, Internal: true}

func FuzzVMRunsVerifiedImages(f *testing.F) {
	for seed := uint64(0); seed < 64; seed++ {
		f.Add(seed, seed%2 == 0)
	}
	f.Fuzz(func(t *testing.T, seed uint64, wide bool) {
		width := 32
		if wide {
			width = 64
		}
		a := vmFuzzImage(t, seed, width)
		bp, err := bytecode.Build(context.Background(), a.image(), bytecode.Options{Limits: bytecode.DefaultLimits()})
		if err != nil {
			return // the generator's stack shape may be rejected; only verified images count
		}
		p, err := NewProgram(bp)
		if err != nil {
			t.Fatalf("NewProgram rejected a verified image: %v", err)
		}
		for _, program := range []*Program{p, fusedProgram(p)} {
			cfg := vmConfig(t, width)
			cfg.Steps, cfg.Depth, cfg.Callbacks, cfg.OutputBytes = 3000, 16, 2000, 1<<16
			res := program.Run(cfg)
			if !vmRunEnds[res.End] {
				t.Fatalf("undeclared end %v", res.End)
			}
			if res.Steps > cfg.Steps {
				t.Fatalf("%d steps beyond the budget %d", res.Steps, cfg.Steps)
			}
			if res.End == Internal && strings.Contains(res.Err.Error(), "panicked") {
				t.Fatalf("panic: %v", res.Err)
			}
			if res.End == Internal {
				t.Fatalf("internal error on a verified image: %v", res.Err)
			}
		}
	})
}
