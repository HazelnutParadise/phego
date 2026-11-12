package runtime

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

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
		// Two element locations. The verifier lets one be used only in the
		// instructions that follow its FetchDim (elementSequence), so no
		// other instruction names them, and no jump lands inside such a
		// sequence (inside).
		elems := []uint32{f.elem(), f.elem()}
		inside := map[uint32]bool{}
		// Two iterator registers that only the iterator instructions write,
		// so they hold arrays; now and then another value stands in, which
		// the instructions must refuse.
		its := []uint32{f.arrayOf(v(), v(), v()), f.arrayOf()}
		iter := func() uint32 {
			if rng.IntN(10) == 0 {
				return v()
			}
			return its[rng.IntN(2)]
		}
		anyLoc := func() uint32 { return f.loc(names[rng.IntN(4)]) }
		// elementSequence is what the compiler emits for an element write
		// of one to three levels: FetchDim from a variable, FetchDim of the
		// element for each further level, then one write through it.
		elementSequence := func(dst uint32) {
			e := elems[rng.IntN(2)]
			bi := rng.IntN(4)
			other := func() uint32 { return f.loc(names[(bi+1+rng.IntN(3))%4]) }
			levels := 1 + rng.IntN(3)
			terminal := rng.IntN(7)
			from := f.loc(names[bi])
			for l := 0; l < levels; l++ {
				// Every mode and use, and an append now and then.
				flags := bytecode.FetchFlags(rng.IntN(3))
				if flags != bytecode.FetchUnset {
					flags |= bytecode.FetchFlags(rng.IntN(3)) << 4
					if rng.IntN(4) == 0 {
						flags |= bytecode.FetchAppend
					}
				}
				key := ""
				if rng.IntN(2) == 0 && flags&bytecode.FetchAppend == 0 {
					key = names[rng.IntN(4)]
				}
				pc := f.pc()
				f.fetch(e, from, flags, key, v())
				if l > 0 {
					inside[pc] = true
				}
				from = e
			}
			mark := f.pc()
			switch terminal {
			case 0:
				f.emit(bytecode.OpStore, 0, e, v(), 0)
			case 1:
				f.emit(bytecode.OpIncDec, uint16(1+rng.IntN(4)), dst, e, 0)
			case 2:
				flags, key, val := bytecode.ArrayWriteFlags(0), v(), v()
				if rng.IntN(3) == 0 {
					flags, key = flags|bytecode.KeyLocation, anyLoc()
				}
				if rng.IntN(3) == 0 {
					flags, val = flags|bytecode.ValueLocation, other()
				}
				aux, ok := bytecode.ArraySetAux(flags, dst)
				if !ok || rng.IntN(3) == 0 {
					aux = uint16(flags)
				}
				f.emit(bytecode.OpArraySet, aux, e, key, val)
			case 3:
				if rng.IntN(2) == 0 {
					f.emit(bytecode.OpArrayAppend, uint16(bytecode.ValueLocation), e, other(), 0)
				} else {
					f.emit(bytecode.OpArrayAppend, 0, e, v(), 0)
				}
			case 4:
				if rng.IntN(2) == 0 {
					f.emit(bytecode.OpArrayUnset, uint16(bytecode.KeyLocation), e, anyLoc(), 0)
				} else {
					f.emit(bytecode.OpArrayUnset, 0, e, v(), 0)
				}
			default:
				// An assign-op: the value variable, the element, the
				// operation, the store.
				rv := v()
				if rng.IntN(2) == 0 {
					f.emit(bytecode.OpLoad, uint16(rng.IntN(2)), rv, other(), 0)
				}
				f.emit(bytecode.OpLoad, 1, dst, e, 0)
				f.emit(bytecode.OpBinary, uint16(1+rng.IntN(20)), dst, dst, rv)
				f.emit(bytecode.OpStore, 0, e, dst, 0)
			}
			for pc := mark; pc < f.pc(); pc++ {
				inside[pc] = true
			}
		}
		start := f.pc()
		n := 1 + rng.IntN(40)
		var jumps []int
		for i := 0; i < n; i++ {
			f.at(uint32(2 + rng.IntN(50)))
			dst := v()
			switch rng.IntN(36) {
			case 25, 35:
				elementSequence(dst)
			case 26:
				// Reads and writes through any Location.
				switch l := anyLoc(); rng.IntN(6) {
				case 0:
					f.emit(bytecode.OpLoad, uint16(rng.IntN(2)), dst, l, 0)
				case 1:
					f.emit(bytecode.OpStore, 0, l, v(), 0)
				case 2:
					f.emit(bytecode.OpIncDec, uint16(1+rng.IntN(4)), dst, l, 0)
				case 3:
					f.emit(bytecode.OpArrayAppend, 0, l, v(), 0)
				case 4:
					f.emit(bytecode.OpArrayUnset, 0, l, v(), 0)
				default:
					f.emit(bytecode.OpIsset, 0, dst, l, 0)
				}
			case 27:
				// ArraySet with a result register, through any Location.
				flags, key, val := bytecode.ArrayWriteFlags(0), v(), v()
				if rng.IntN(3) == 0 {
					flags, key = flags|bytecode.KeyLocation, f.loc(names[rng.IntN(4)])
				}
				if rng.IntN(3) == 0 {
					flags, val = flags|bytecode.ValueLocation, f.loc(names[rng.IntN(4)])
				}
				aux, ok := bytecode.ArraySetAux(flags, dst)
				if !ok || rng.IntN(3) == 0 {
					aux = uint16(flags)
				}
				f.emit(bytecode.OpArraySet, aux, anyLoc(), key, val)
			case 28, 29:
				// IterInit clears its iterator when the subject cannot be
				// iterated; that edge lands on an instruction that defines
				// the register again, so every later read is verified.
				it := iter()
				pc := f.emit(bytecode.OpIterInit, 0, it, v(), 0)
				f.jump(f.pc() + 2)
				f.code[pc].C = f.pc()
				f.emit(bytecode.OpNewArray, 0, it, 0, 0)
			case 30, 31:
				// IterNext on any value: an array iterates from wherever its
				// position stands, anything else is refused.
				it := iter()
				if rng.IntN(2) == 0 {
					jumps = append(jumps, f.emit(bytecode.OpIterNext, 0, it, dst, 0))
				} else {
					jumps = append(jumps, f.emit(bytecode.OpIterNext, uint16(bytecode.IterLocation), it, anyLoc(), 0))
				}
				// Usually the key of the element just fetched.
				if rng.IntN(3) != 0 {
					if rng.IntN(2) == 0 {
						f.emit(bytecode.OpIterKey, 0, v(), it, 0)
					} else {
						f.emit(bytecode.OpIterKey, uint16(bytecode.IterLocation), anyLoc(), it, 0)
					}
				}
			case 32:
				// A key read wherever control arrives from: refused when
				// the iterator has fetched nothing yet.
				if rng.IntN(4) == 0 {
					f.emit(bytecode.OpIterKey, 0, dst, iter(), 0)
				}
			case 33:
				if f.params > 0 {
					jumps = append(jumps, f.emit(bytecode.OpParamDefault, 0, uint32(rng.IntN(f.params)), 0, 0))
				}
			case 34:
				// A string to read and write offsets of.
				f.store(names[rng.IntN(4)], f.k([]string{"", "abc", "0123456789"}[rng.IntN(3)]))
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
				// Mostly the literal under construction, as compiled code
				// does; another value is refused.
				arr := its[rng.IntN(2)]
				if rng.IntN(8) == 0 {
					arr = v()
				}
				if rng.IntN(2) == 0 {
					f.emit(bytecode.OpArrayAdd, 0, arr, v(), 0)
				} else {
					f.add(arr, v(), v())
				}
			case 21:
				f.addVarKey(iter(), names[rng.IntN(4)], v())
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
			case 24:
				f.unsetVar(names[rng.IntN(4)], names[rng.IntN(4)])
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
		pick := func() uint32 {
			for {
				if pc := start + uint32(rng.IntN(int(end-start)+1)); !inside[pc] {
					return pc
				}
			}
		}
		for _, pc := range jumps {
			target, other := pick(), pick()
			switch f.code[pc].Op {
			case bytecode.OpIterNext:
				f.code[pc].C = target
			case bytecode.OpParamDefault:
				f.code[pc].B = target
			default:
				f.patch(pc, target, other)
			}
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
			// One step may copy or pad a string of up to 128 MiB, so a
			// run of 3000 steps can take seconds: a deadline bounds it,
			// and the copies must observe it.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			cfg.Context = ctx
			res := program.Run(cfg)
			cancel()
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

// The generator must keep producing images the verifier accepts and runs
// that get past their first instructions, or the fuzz target tests nothing.
func TestVMFuzzGeneratorCoversStage2(t *testing.T) {
	const seeds = 400
	built := 0
	ops := map[bytecode.Opcode]int{}
	ends := map[RunEnd]int{}
	refused := map[string]int{}
	for seed := uint64(0); seed < seeds; seed++ {
		a := vmFuzzImage(t, seed, 64)
		img := a.image()
		bp, err := bytecode.Build(context.Background(), img, bytecode.Options{Limits: bytecode.DefaultLimits()})
		if err != nil {
			continue
		}
		built++
		for _, f := range img.Functions {
			for _, in := range f.Code {
				ops[in.Op]++
			}
		}
		p, err := NewProgram(bp)
		if err != nil {
			t.Fatalf("seed %d: NewProgram rejected a verified image: %v", seed, err)
		}
		cfg := vmConfig(t, 64)
		cfg.Steps, cfg.Depth, cfg.Callbacks, cfg.OutputBytes = 3000, 16, 2000, 1<<16
		res := p.Run(cfg)
		ends[res.End]++
		if res.End == Internal {
			t.Fatalf("seed %d: internal error on a verified image: %v", seed, res.Err)
		}
		var u *UnsupportedError
		if errors.As(res.Err, &u) {
			refused[u.Feature]++
		}
	}
	t.Logf("refused: %v", refused)
	if built < seeds*6/10 {
		t.Fatalf("only %d of %d generated images verify", built, seeds)
	}
	for _, op := range []bytecode.Opcode{bytecode.OpFetchDim, bytecode.OpIterInit, bytecode.OpIterNext, bytecode.OpIterKey, bytecode.OpParamDefault} {
		if ops[op] < 50 {
			t.Errorf("opcode %d appears %d times in the verified images", op, ops[op])
		}
	}
	if ends[Unsupported] > built/4 || ends[Completed]+ends[UncaughtThrowable]+ends[Exited]+ends[LimitExceeded] < built*2/3 {
		t.Errorf("runs end as %v: too few get past the refused instructions", ends)
	}
	t.Logf("%d of %d images verify; ends %v", built, seeds, ends)
}
