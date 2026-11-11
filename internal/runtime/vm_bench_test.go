package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 layout benchmarks. Each candidate runs the same image and must
// produce the same output, return value and step count (checked before
// timing), so only the interpreter layout differs. Every op is one complete
// Run, including request and execution setup.

const vmBenchN = 2000

// forLoop emits for ($i = 0; $i < n; $i = $i + 1) { body }.
func (f *vmFn) forLoop(i string, n int, body func()) {
	f.store(i, f.k(0))
	head := f.pc()
	f.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	br := f.branch(f.bin(bytecode.Less, f.load(i), f.k(n)))
	start := f.pc()
	body()
	f.store(i, f.bin(bytecode.Add, f.load(i), f.k(1)))
	f.jump(head)
	f.patch(br, start, f.pc())
}

var vmWorkloads = []struct {
	name  string
	build func(a *vmAsm)
}{
	{"arith", func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "i", "s")
		m.store("s", m.k(0))
		m.forLoop("i", vmBenchN, func() {
			m.store("s", m.bin(bytecode.Add, m.load("s"), m.bin(bytecode.Multiply, m.load("i"), m.k(2))))
		})
		m.ret(m.load("s"))
	}},
	{"branch", func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "i", "c")
		m.store("c", m.k(0))
		m.forLoop("i", vmBenchN, func() {
			// if ($i % 3 == 0) $c = $c + 1; elseif ($i % 5 < 2) $c = $c - 1; else $c = $c + 2;
			b1 := m.branch(m.bin(bytecode.Equal, m.bin(bytecode.Modulo, m.load("i"), m.k(3)), m.k(0)))
			t1 := m.pc()
			m.store("c", m.bin(bytecode.Add, m.load("c"), m.k(1)))
			j1 := m.jump(0)
			e1 := m.pc()
			b2 := m.branch(m.bin(bytecode.Less, m.bin(bytecode.Modulo, m.load("i"), m.k(5)), m.k(2)))
			t2 := m.pc()
			m.store("c", m.bin(bytecode.Subtract, m.load("c"), m.k(1)))
			j2 := m.jump(0)
			e2 := m.pc()
			m.store("c", m.bin(bytecode.Add, m.load("c"), m.k(2)))
			end := m.pc()
			m.patch(b1, t1, e1)
			m.patch(b2, t2, e2)
			m.patch(j1, end, 0)
			m.patch(j2, end, 0)
		})
		m.ret(m.load("c"))
	}},
	{"concat", func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "i", "s")
		m.store("s", m.k(""))
		m.forLoop("i", vmBenchN/4, func() {
			m.store("s", m.bin(bytecode.Concat, m.bin(bytecode.Concat, m.load("s"), m.k("ab")), m.load("i")))
		})
		m.echo(m.load("s"))
		m.retNull()
	}},
	{"fib", func(a *vmAsm) {
		m := a.fn("{main}", 0, 2)
		m.ret(m.call(1, m.k(18)))
		f := a.fn("fib", 1, 3, "n")
		br := f.branch(f.bin(bytecode.Less, f.load("n"), f.k(2)))
		f.patch(br, f.pc(), f.pc()+2)
		f.ret(f.load("n"))
		f.ret(f.bin(bytecode.Add, f.call(1, f.bin(bytecode.Subtract, f.load("n"), f.k(1))), f.call(1, f.bin(bytecode.Subtract, f.load("n"), f.k(2)))))
	}},
	{"array", func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "i", "a", "s")
		m.store("a", m.newArray())
		m.store("s", m.k(0))
		m.forLoop("i", vmBenchN, func() { m.push("a", m.load("i")) })
		m.forLoop("i", vmBenchN, func() {
			m.store("s", m.bin(bytecode.Add, m.load("s"), m.get(m.load("a"), m.load("i"), false)))
			// $a[$i] = $s: a write after a read of the same array.
			m.set("a", m.load("i"), m.load("s"))
		})
		m.ret(m.load("s"))
	}},
}

type vmCandidate struct {
	name    string
	variant vmVariant
}

func benchCandidates(b *testing.B, candidates []vmCandidate) {
	for _, w := range vmWorkloads {
		a := newAsm(b, 64)
		w.build(a)
		p := a.program()
		var ref RunResult
		for i, c := range candidates {
			cfg := vmConfig(b, 64)
			cfg.variant = c.variant
			res := p.Run(cfg)
			if res.End != Completed {
				b.Fatalf("%s/%s: %v %v", w.name, c.name, res.End, res.Err)
			}
			if i == 0 {
				ref = res
			} else if string(res.Output) != string(ref.Output) || encodeSnapshot(res.Return) != encodeSnapshot(ref.Return) || res.Steps != ref.Steps {
				b.Fatalf("%s/%s differs from %s: steps %d vs %d", w.name, c.name, candidates[0].name, res.Steps, ref.Steps)
			}
		}
		for _, c := range candidates {
			b.Run(w.name+"/"+c.name, func(b *testing.B) {
				b.ReportAllocs()
				req := make([]*Request, b.N)
				for i := range req {
					req[i] = readyRequest(b)
				}
				cfg := vmConfig(b, 64)
				cfg.variant = c.variant
				// Hosts pass cancellable contexts; Background has no Done channel
				// and would make polling free.
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cfg.Context = ctx
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cfg.Request = req[i]
					p.Run(cfg)
				}
				b.ReportMetric(float64(ref.Steps), "steps/op")
			})
		}
	}
}

func BenchmarkVMDispatch(b *testing.B) {
	benchCandidates(b, []vmCandidate{{"switch", vmVariant{}}, {"closures", vmVariant{dispatch: 1}}, {"fused", vmVariant{dispatch: 2}}})
}

func BenchmarkVMFrames(b *testing.B) {
	benchCandidates(b, []vmCandidate{{"slab", vmVariant{}}, {"fresh", vmVariant{frames: 1}}, {"perdepth", vmVariant{frames: 2}}})
}

func BenchmarkVMPolling(b *testing.B) {
	benchCandidates(b, []vmCandidate{{"backedge1024", vmVariant{}}, {"every", vmVariant{polling: 1}}})
}

// BenchmarkVMLocals compares frame-owned local slots with registering each
// slot through execution.local, which scans every frame's locals, on a
// call-heavy program with eight locals per frame.
func BenchmarkVMLocals(b *testing.B) {
	a := newAsm(b, 64)
	m := a.fn("{main}", 0, 2, "i")
	m.forLoop("i", 200, func() { m.call(1, m.k(6)) })
	m.retNull()
	f := a.fn("rec", 1, 3, "n", "a", "b", "c", "d", "e", "g", "h")
	f.store("a", f.load("n"))
	br := f.branch(f.bin(bytecode.Less, f.load("n"), f.k(1)))
	f.patch(br, f.pc(), f.pc()+2)
	f.ret(f.load("a"))
	f.ret(f.call(1, f.bin(bytecode.Subtract, f.load("n"), f.k(1))))
	p := a.program()
	for _, c := range []vmCandidate{{"slots", vmVariant{}}, {"registered", vmVariant{locals: 1}}} {
		cfg := vmConfig(b, 64)
		cfg.variant = c.variant
		if res := p.Run(cfg); res.End != Completed {
			b.Fatalf("%s: %v %v", c.name, res.End, res.Err)
		}
		b.Run(c.name, func(b *testing.B) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg.Context = ctx
			b.ReportAllocs()
			req := make([]*Request, b.N)
			for i := range req {
				req[i] = readyRequest(b)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cfg.Request = req[i]
				p.Run(cfg)
			}
		})
	}
}

// The closure dispatch candidate: one pre-decoded Go closure per
// instruction, covering the opcodes of the benchmark workloads.
func init() {
	vmClosureCandidate = &struct {
		build func(*Program) [][]vmClosure
		loop  func(*vmState)
	}{build: buildClosures, loop: loopClosures}
}

const (
	closureStop   = -1
	closureSwitch = -2
)

func loopClosures(s *vmState) {
	f := &s.frames[len(s.frames)-1]
	cl := s.closures[f.fn.index]
	pc := 0
	for {
		f.pc = pc
		s.steps++
		if s.steps >= s.next && s.slow() {
			return
		}
		next := cl[pc](s, f)
		if next >= 0 {
			pc = next
			continue
		}
		if next == closureStop || s.ended {
			return
		}
		f = &s.frames[len(s.frames)-1]
		cl = s.closures[f.fn.index]
		pc = f.pc
	}
}

func buildClosures(p *Program) [][]vmClosure {
	out := make([][]vmClosure, len(p.funcs))
	for fi := range p.funcs {
		fn := &p.funcs[fi]
		cl := make([]vmClosure, len(fn.code))
		for pc := range fn.code {
			cl[pc] = closureFor(p, fn.code[pc], pc)
		}
		out[fi] = cl
	}
	return out
}

func closureFor(p *Program, in vmInstr, pc int) vmClosure {
	next := pc + 1
	tail := func(s *vmState, f *vmFrame, err error) int {
		if err != nil {
			s.stop(err)
			return closureStop
		}
		if s.stopped() {
			return closureStop
		}
		if in.nkill != 0 {
			if err := s.kill(f, &in); err != nil {
				s.stop(err)
				return closureStop
			}
		}
		return next
	}
	switch in.op {
	case bytecode.OpCheckpoint:
		return func(s *vmState, f *vmFrame) int {
			if s.poll() {
				return closureStop
			}
			return next
		}
	case bytecode.OpConst:
		c := p.consts[in.b]
		return func(s *vmState, f *vmFrame) int { return tail(s, f, s.setReg(f.regs, in.a, c)) }
	case bytecode.OpLocal:
		return func(s *vmState, f *vmFrame) int { f.locs[in.a] = int32(in.b) + 1; return next }
	case bytecode.OpLoad:
		return func(s *vmState, f *vmFrame) int {
			slot, err := s.slot(f, in.b)
			if err != nil {
				return tail(s, f, err)
			}
			v := slot.deref()
			if v.kind == valueUndefined {
				if in.aux == 0 {
					s.h.warn(levelWarning, "Undefined variable $"+s.localName(f, in.b))
				}
				v = nullValue()
			}
			return tail(s, f, s.setReg(f.regs, in.a, v))
		}
	case bytecode.OpStore:
		return func(s *vmState, f *vmFrame) int {
			slot, err := s.slot(f, in.a)
			if err != nil {
				return tail(s, f, err)
			}
			v := f.regs[in.b]
			if slot.kind < valueArray && v.kind < valueArray && v.kind != valueUndefined {
				*slot = v
				return next
			}
			if v.kind == valueUndefined {
				v = nullValue()
			}
			return tail(s, f, s.h.assign(slot, v))
		}
	case bytecode.OpBinary:
		op := bytecode.BinaryOp(in.aux)
		return func(s *vmState, f *vmFrame) int {
			r, ok := s.fastBinary(op, f.regs[in.b], f.regs[in.c])
			if !ok {
				var err error
				if r, err = s.binary(op, f.regs[in.b], f.regs[in.c]); err != nil {
					return tail(s, f, err)
				}
			}
			return tail(s, f, s.setReg(f.regs, in.a, r))
		}
	case bytecode.OpJump:
		target := int(in.a)
		return func(s *vmState, f *vmFrame) int {
			if target <= pc && s.poll() {
				return closureStop
			}
			return target
		}
	case bytecode.OpBranch:
		return func(s *vmState, f *vmFrame) int {
			cond := s.h.toBool(f.regs[in.a])
			if r := tail(s, f, nil); r < 0 {
				return r
			}
			target := int(in.c)
			if cond {
				target = int(in.b)
			}
			if target <= pc && s.poll() {
				return closureStop
			}
			return target
		}
	case bytecode.OpPush:
		return func(s *vmState, f *vmFrame) int {
			v := f.regs[in.a]
			if v.kind >= valueArray {
				retain(v)
			}
			s.args = append(s.args, v)
			return tail(s, f, nil)
		}
	case bytecode.OpCall:
		return func(s *vmState, f *vmFrame) int {
			if err := s.push(&s.prog.funcs[in.b], int(in.c), in.a); err != nil {
				s.stop(err)
				return closureStop
			}
			if s.poll() {
				return closureStop
			}
			return closureSwitch
		}
	case bytecode.OpReturn:
		return func(s *vmState, f *vmFrame) int {
			v := take(f.regs, in.a)
			if len(s.frames) == 1 {
				s.ended = true
				s.end = completion{kind: endReturn, value: v, returnOwned: true}
				return closureStop
			}
			if err := s.ret(v); err != nil {
				s.stop(err)
				return closureStop
			}
			if s.poll() {
				return closureStop
			}
			s.frames[len(s.frames)-1].pc++
			return closureSwitch
		}
	case bytecode.OpOutput:
		return func(s *vmState, f *vmFrame) int {
			if c := s.output(f.regs[in.a]); c.kind != endNormal {
				s.stop(c)
				return closureStop
			}
			return tail(s, f, nil)
		}
	case bytecode.OpNewArray:
		return func(s *vmState, f *vmFrame) int { return tail(s, f, s.setReg(f.regs, in.a, s.h.newArrayValue())) }
	case bytecode.OpArrayGet:
		return func(s *vmState, f *vmFrame) int { return tail(s, f, s.arrayGet(f.regs, &in)) }
	case bytecode.OpArrayAppend:
		return func(s *vmState, f *vmFrame) int { return tail(s, f, s.arrayAppend(f, &in)) }
	case bytecode.OpArraySet:
		return func(s *vmState, f *vmFrame) int { return tail(s, f, s.arraySet(f, &in)) }
	}
	return func(s *vmState, f *vmFrame) int {
		s.stop(s.unsupported(fmt.Sprintf("opcode %d in the closure candidate", in.op)))
		return closureStop
	}
}

// TestVMCandidatesAgree runs every workload under every candidate once, so
// the candidates stay equivalent outside benchmark runs too.
func TestVMCandidatesAgree(t *testing.T) {
	variants := []vmVariant{{}, {dispatch: 1}, {dispatch: 2}, {frames: 1}, {frames: 2}, {locals: 1}, {polling: 1}}
	for _, w := range vmWorkloads {
		a := newAsm(t, 64)
		w.build(a)
		p := a.program()
		var ref RunResult
		for i, v := range variants {
			cfg := vmConfig(t, 64)
			cfg.variant = v
			res := p.Run(cfg)
			if res.End != Completed {
				t.Fatalf("%s %+v: %v %v", w.name, v, res.End, res.Err)
			}
			if i == 0 {
				ref = res
				continue
			}
			if string(res.Output) != string(ref.Output) || encodeSnapshot(res.Return) != encodeSnapshot(ref.Return) || res.Steps != ref.Steps {
				t.Fatalf("%s %+v differs", w.name, v)
			}
		}
		t.Logf("%s: %d steps, return %s", w.name, ref.Steps, encodeSnapshot(ref.Return))
	}
}
