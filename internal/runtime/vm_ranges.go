package runtime

import (
	"sort"

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
