package runtime

import (
	"sort"
	"sync"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026. PHP frees a temporary
// at its last use in code order and, when an exception or exit leaves the
// code between its definition and that use, during the unwinding
// (zend_calc_live_ranges, cleanup_live_vars). The VM releases a register
// at its last use on any path (vmLiveness); the ranges here are PHP's view
// of the same registers.

// vmRange is a live range in PHP's sense: register reg holds a value
// defined at start that is used again at end, and nothing after start
// redefines it before end.
type vmRange struct {
	start, end int32
	reg        uint32
}

// vmLinearRanges computes the live ranges of the composite registers of f
// as zend_calc_live_ranges does: scanning the code backwards, a use opens
// a range at its pc unless one is open already, and a definition closes
// it. A range whose only use is the next instruction is left out, as PHP
// leaves out trivial ranges. The ranges are sorted by start.
func vmLinearRanges(f *vmFunction, composite []bool, uses func(vmInstr, func(uint32)), def func(vmInstr) (uint32, bool)) []vmRange {
	last := make([]int32, f.nvals)
	for i := range last {
		last[i] = -1
	}
	var out []vmRange
	for pc := len(f.code) - 1; pc >= 0; pc-- {
		in := f.code[pc]
		if d, ok := def(in); ok && composite[d] && last[d] >= 0 {
			if int32(pc)+1 != last[d] {
				out = append(out, vmRange{start: int32(pc), end: last[d], reg: d})
			}
			last[d] = -1
		}
		uses(in, func(r uint32) {
			if composite[r] && last[r] < 0 {
				last[r] = int32(pc)
			}
		})
	}
	// Emitted backwards: reversed, they are in order of start except where
	// ranges nest, which the stable sort settles.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// vmTerminalHolds lists, for each instruction that ends the code's path
// without a successor in the function (exit, throw, an unmatched match),
// the registers whose PHP live range spans it.
func vmTerminalHolds(f *vmFunction, composite []bool, uses func(vmInstr, func(uint32)), def func(vmInstr) (uint32, bool)) map[int32][]uint32 {
	var terms []int32
	for pc, in := range f.code {
		switch in.op {
		case bytecode.OpExit, bytecode.OpThrow, bytecode.OpMatchError:
			terms = append(terms, int32(pc))
		}
	}
	if len(terms) == 0 {
		return nil
	}
	holds := map[int32][]uint32{}
	for _, r := range vmLinearRanges(f, composite, uses, def) {
		i := sort.Search(len(terms), func(i int) bool { return terms[i] > r.start })
		for ; i < len(terms) && terms[i] < r.end; i++ {
			holds[terms[i]] = append(holds[terms[i]], r.reg)
		}
	}
	return holds
}

// vmRangeCache holds a function's live ranges by register, computed the
// first time an exception or exit unwinds the function. Functions are
// shared by concurrent runs, so it is filled once.
type vmRangeCache struct {
	once  sync.Once
	byReg [][]vmRange
}

// rangesFor returns PHP's live ranges of f's composite registers, by
// register and in order of start, or nil for a function without them.
func (f *vmFunction) rangesFor() [][]vmRange {
	if f.ranges == nil {
		return nil
	}
	f.ranges.once.Do(func() {
		byReg := make([][]vmRange, f.nvals)
		for _, r := range vmLinearRanges(f, vmComposite(f), vmUses, vmDef) {
			byReg[r.reg] = append(byReg[r.reg], r)
		}
		f.ranges.byReg = byReg
	})
	return f.ranges.byReg
}

// unwindKey orders register r of frame f at pc by the start of the PHP live
// range that holds it there: a return that freed it as a loop variable
// before pc started that range again (UnwindSplit), at its position among
// the variables that return freed. A register no range holds comes last,
// in register order.
func unwindKey(fn *vmFunction, byReg [][]vmRange, r uint32, pc int32) uint64 {
	const tail = uint64(1) << 62
	if byReg == nil || int(r) >= len(byReg) {
		return tail + uint64(r)
	}
	for _, rg := range byReg[r] {
		if rg.start > pc {
			break
		}
		if pc >= rg.end {
			continue
		}
		key := uint64(rg.start) << 17
		at := rg.start
		for _, u := range fn.splits {
			if u.Reg == r && int32(u.Start) > at && int32(u.Start) < pc {
				at = int32(u.Start)
				key = uint64(u.Start+1)<<17 | 1<<16 | uint64(u.Count)
			}
		}
		return key
	}
	return tail + uint64(r)
}

// releaseUnwound frees what frame f, the top frame, holds at its pc when an
// exception or exit leaves that code, in PHP's order: the operands of the
// instruction that failed (its FREE_OP), the calls it left unfinished,
// innermost first (cleanup_unfinished_calls), the temporaries in the order
// their live ranges start (cleanup_live_vars), and then whatever else the
// registers hold. With all the frame ends and every register goes;
// otherwise a handler of the frame takes over and the registers of live,
// which it still uses, are kept.
func (s *vmState) releaseUnwound(f *vmFrame, live []uint32, all bool, keep func(error)) {
	fn := f.fn
	pc := f.pc
	depth := len(s.frames)
	kept := func(r uint32) bool {
		if all {
			return false
		}
		if !fn.liveKnown {
			// Without liveness facts the registers stay until they are
			// overwritten.
			return true
		}
		i := sort.Search(len(live), func(i int) bool { return live[i] >= r })
		return i < len(live) && live[i] == r
	}
	free := func(r uint32) {
		if int(r) >= len(f.regs) || f.regs[r].kind < valueArray || kept(r) {
			return
		}
		v := f.regs[r]
		f.regs[r] = value{}
		keep(s.releaseValue(v))
	}
	if pc < len(f.code) {
		vmUses(f.code[pc], free)
	}
	if len(fn.calls) > 0 {
		// The innermost call that started at or before pc, then the calls
		// around it; those that ended before pc are not unfinished.
		i := sort.Search(len(fn.calls), func(i int) bool { return int(fn.calls[i].start) > pc }) - 1
		for i >= 0 {
			c := &fn.calls[i]
			if pc < int(c.end) {
				if c.pending {
					if n := len(s.calls); n > 0 && s.calls[n-1].depth >= depth {
						s.releaseCall(keep)
					}
				} else {
					for _, r := range c.args {
						free(r)
					}
				}
			}
			i = int(c.parent)
		}
	}
	for n := len(s.calls); n > 0 && s.calls[n-1].depth >= depth; n = len(s.calls) {
		s.releaseCall(keep)
	}
	var cand []uint32
	for r := range f.regs {
		if f.regs[r].kind >= valueArray && !kept(uint32(r)) {
			cand = append(cand, uint32(r))
		}
	}
	if len(cand) > 1 {
		byReg := fn.rangesFor()
		keys := make([]uint64, len(f.regs))
		for _, r := range cand {
			keys[r] = unwindKey(fn, byReg, r, int32(pc))
		}
		sort.SliceStable(cand, func(i, j int) bool { return keys[cand[i]] < keys[cand[j]] })
	}
	for _, r := range cand {
		free(r)
	}
}

// releaseCall frees the innermost call InitCall, InitMethod, InitStatic or
// New started and no CallPending made: its arguments in order, its object,
// the named arguments its variadic parameter collects and its Closure, as
// cleanup_unfinished_calls does.
func (s *vmState) releaseCall(keep func(error)) {
	n := len(s.calls)
	c := s.calls[n-1]
	s.calls[n-1] = vmPendingCall{}
	s.calls = s.calls[:n-1]
	if c.base < len(s.args) {
		args := append([]value(nil), s.args[c.base:]...)
		clear(s.args[c.base:])
		s.args = s.args[:c.base]
		for _, v := range args {
			keep(s.releaseValue(v))
		}
	}
	if c.this != nil {
		if c.ctor {
			c.this.destructorCalled = true
		}
		keep(s.releaseObject(c.this))
	}
	keep(s.releaseValue(c.named))
	if c.closure != nil {
		keep(s.releaseObject(c.closure))
	}
}

// vmUnwindCall is a call of a function's unwind table: unfinished on
// [start, end), with its arguments in the registers args (UnwindCall) or on
// the argument stack (pending). parent is the index of the call it is
// nested in, or -1.
type vmUnwindCall struct {
	start, end int32
	pending    bool
	args       []uint32
	parent     int32
}

// addUnwind adds entry u of the verified unwind table, which comes sorted
// by start with the calls nested or disjoint.
func (f *vmFunction) addUnwind(u bytecode.Unwind) {
	if u.Kind == bytecode.UnwindSplit {
		f.splits = append(f.splits, u)
		return
	}
	start, end := int32(u.Start), int32(u.End)
	if n := len(f.calls); n > 0 && f.calls[n-1].start == start && f.calls[n-1].end == end && !f.calls[n-1].pending && u.Kind == bytecode.UnwindCall {
		f.calls[n-1].args = append(f.calls[n-1].args, u.Reg)
		return
	}
	c := vmUnwindCall{start: start, end: end, pending: u.Kind == bytecode.UnwindPendingCall, parent: -1}
	if u.Kind == bytecode.UnwindCall {
		c.args = []uint32{u.Reg}
	}
	// The call around this one: the latest earlier call that has not
	// ended at start, found through the parents of the previous call.
	for p := int32(len(f.calls) - 1); p >= 0; p = f.calls[p].parent {
		if f.calls[p].end > start {
			c.parent = p
			break
		}
	}
	f.calls = append(f.calls, c)
}
