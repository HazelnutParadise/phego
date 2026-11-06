package ssa

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
)

var testProfile = profile.Profile{Release: "8.5.12", IntWidth: 64}

func lowerSource(t *testing.T, src string, width int) (*ir.Unit, []byte) {
	t.Helper()
	bytes := []byte(src)
	tree, err := frontend.Parse(bytes, frontend.LexOptions{IntWidth: width, MaxBytes: len(bytes) + 1})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := testProfile
	p.IntWidth = width
	u, err := ir.Lower(tree, p, ir.Options{})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return u, bytes
}

func buildSource(t *testing.T, src string, width int, passes Passes) *Program {
	t.Helper()
	u, _ := lowerSource(t, src, width)
	p, err := Build(context.Background(), u, Options{Passes: passes, MaxNodes: 10000, MaxDepth: 128, MaxConstantBytes: 1 << 20})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return p
}

func valuesForHIR(p *Program, want *ir.Node) []Value {
	var out []Value
	for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
		v, _ := p.Value(id)
		if v.HIR == want {
			out = append(out, v)
		}
	}
	return out
}

func walkHIR(n *ir.Node, visit func(*ir.Node)) {
	if n == nil {
		return
	}
	visit(n)
	for _, k := range n.Kids {
		walkHIR(k, visit)
	}
	for _, a := range n.Args {
		if a != nil {
			walkHIR(a.Value, visit)
		}
	}
}

func TestConstantFoldingAndTargetIntegerWidths(t *testing.T) {
	p := buildSource(t, "<?php 1+2*3;", 64, AllPasses)
	var found bool
	for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
		v, _ := p.Value(id)
		if v.HIR.Op == ir.OpBinary && v.HIR.Text == "+" {
			found = true
			if !v.IsConstant || v.Constant.Kind != ir.LitInt || v.Constant.Int != 7 {
				t.Fatalf("1+2*3 = %#v", v)
			}
		}
	}
	if !found {
		t.Fatal("addition node absent")
	}
	for _, tc := range []struct {
		width  int
		expr   string
		folded bool
		value  int64
	}{
		{32, "2147483647+1", false, 0}, {64, "2147483647+1", true, 2147483648},
		{64, "9223372036854775807+1", false, 0},
	} {
		p := buildSource(t, "<?php "+tc.expr+";", tc.width, FoldConstants)
		var got *Value
		for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
			v, _ := p.Value(id)
			if v.HIR.Op == ir.OpBinary {
				vv := v
				got = &vv
			}
		}
		if got == nil || got.IsConstant != tc.folded || tc.folded && got.Constant.Int != tc.value {
			t.Errorf("%d-bit %s => %#v", tc.width, tc.expr, got)
		}
	}
}

func TestExactStringBoolAndBitwiseFolding(t *testing.T) {
	for _, tc := range []struct {
		src  string
		op   ir.Op
		text string
		kind ir.LiteralKind
		str  string
		b    bool
		i    int64
	}{
		{"<?php 'a'.'b';", ir.OpBinary, ".", ir.LitString, "ab", false, 0},
		{"<?php !true;", ir.OpUnary, "!", ir.LitBool, "", false, 0},
		{"<?php 6|3;", ir.OpBinary, "|", ir.LitInt, "", false, 7},
	} {
		p := buildSource(t, tc.src, 64, AllPasses)
		var matched bool
		for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
			v, _ := p.Value(id)
			if v.HIR.Op == tc.op && v.HIR.Text == tc.text {
				matched = true
				if !v.IsConstant || v.Constant.Kind != tc.kind || tc.kind == ir.LitString && v.Constant.Str != tc.str || tc.kind == ir.LitBool && v.Constant.Bool != tc.b || tc.kind == ir.LitInt && v.Constant.Int != tc.i {
					t.Errorf("%s: %#v", tc.src, v)
				}
			}
		}
		if !matched {
			t.Errorf("node absent for %s", tc.src)
		}
	}
}

func TestSpecializationRequiresProvenOperandTypes(t *testing.T) {
	p := buildSource(t, "<?php !isset($x); ((int)$a)|((int)$b); $a|$b;", 64, AllPasses)
	var boolNot, bitwise, unknownBitwise bool
	for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
		v, _ := p.Value(id)
		if v.HIR.Op == ir.OpUnary && v.HIR.Text == "!" {
			boolNot = v.Specialization == BoolNot
		}
		if v.HIR.Op == ir.OpBinary && v.HIR.Text == "|" {
			if v.HIR.Kids[0].Op == ir.OpCast {
				bitwise = v.Specialization == IntBitwise
			}
			if v.HIR.Kids[0].Op == ir.OpVar && v.Specialization != Generic {
				unknownBitwise = true
			}
		}
	}
	if !boolNot || !bitwise || unknownBitwise {
		t.Fatalf("boolNot=%t bitwise=%t unknown special=%t", boolNot, bitwise, unknownBitwise)
	}
}

func TestOrderedOperationsAndLocationsRemain(t *testing.T) {
	u, _ := lowerSource(t, "<?php function f(&$x) { global $g; extract($a); compact('x'); $$x; $x(); unknown_op(); }", 64)
	p, err := Build(context.Background(), u, Options{Passes: AllPasses, MaxNodes: 10000, MaxDepth: 128, MaxConstantBytes: 10000})
	if err != nil {
		t.Fatal(err)
	}
	var byref, dynamic, compact, varvar, call bool
	for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
		v, _ := p.Value(id)
		if v.Effects.Ordered {
			switch v.HIR.Op {
			case ir.OpVar:
				if v.Effects.Ordered && v.HIR.Var != nil && v.HIR.Var.Flags.Aliased() {
					byref = true
				}
			case ir.OpVarVar:
				varvar = true
			case ir.OpCall:
				call = true
			}
		}
		if v.HIR.Op == ir.OpCall && v.HIR.Name != nil {
			if strings.EqualFold(v.HIR.Name.Resolved, "extract") {
				dynamic = v.Effects.Ordered
			}
			if strings.EqualFold(v.HIR.Name.Resolved, "compact") {
				compact = v.Effects.Ordered
			}
		}
	}
	if !byref || !dynamic || !compact || !varvar || !call {
		t.Fatalf("ordered coverage byref=%t extract=%t compact=%t varvar=%t call=%t", byref, dynamic, compact, varvar, call)
	}
}

func TestUnknownAndRuntimeGuardsStayOrdered(t *testing.T) {
	main := &ir.Func{}
	for _, op := range []ir.Op{"unknown_op", "policy_check", "cancel_check", "quota_charge"} {
		main.Body = append(main.Body, &ir.Node{Op: op})
	}
	p, err := Build(context.Background(), &ir.Unit{Profile: testProfile, Main: main}, Options{Passes: AllPasses, MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range p.functions[0].roots {
		v, ok := p.Value(root)
		if !ok || !v.Effects.Ordered {
			t.Errorf("%s lost ordered barrier: %#v", v.HIR.Op, v)
		}
	}
}

func TestClosureAndClassInitializerAreBuilt(t *testing.T) {
	closure := &ir.Func{Kind: ir.FuncClosure, Body: []*ir.Node{{Op: ir.OpReturn}}}
	closureNode := &ir.Node{Op: ir.OpClosure, Func: closure}
	init := &ir.Node{Op: ir.OpLiteral, Value: &ir.Literal{Kind: ir.LitInt, Int: 9}}
	u := &ir.Unit{Profile: testProfile, Main: &ir.Func{Body: []*ir.Node{closureNode}}, Classes: []*ir.Class{{Consts: []*ir.ClassConst{{Name: "C", Value: init}}}}}
	p, err := Build(context.Background(), u, Options{MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if p.FunctionCount() != 2 {
		t.Fatalf("closure function missing: count=%d", p.FunctionCount())
	}
	if len(p.Initializers()) != 1 {
		t.Fatalf("class initializer missing: %v", p.Initializers())
	}
	v, ok := p.Value(p.Initializers()[0])
	if !ok || v.HIR != init {
		t.Fatalf("initializer value mismatch: %#v", v)
	}
}

func TestHIRIdentityEdgesAndDistinctOccurrences(t *testing.T) {
	u, _ := lowerSource(t, "<?php $x + $x;", 64)
	n := u.Main.Body[0].Kids[0]
	before := *n
	p, err := Build(context.Background(), u, Options{Passes: 0, MaxNodes: 1000, MaxDepth: 64, MaxConstantBytes: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, *n) {
		t.Fatal("Build mutated input HIR")
	}
	ids := valuesForHIR(p, n)
	if len(ids) != 1 {
		t.Fatalf("node records: %d", len(ids))
	}
	root := p.functions[0].roots[0]
	rootID := p.Child(root, 0)
	if rootID == None || p.Child(rootID, 0) == p.Child(rootID, 1) {
		t.Fatal("repeated HIR occurrence reused a node ID")
	}
	if p.Child(root, -1) != None || p.Child(root, 999) != None || p.Argument(root, 0) != None {
		t.Fatal("invalid edge lookup should return None")
	}
}

func TestChildrenArgumentsFunctionsDefaultsAndNilSlots(t *testing.T) {
	leafA := &ir.Node{Op: ir.OpLiteral, Value: &ir.Literal{Kind: ir.LitInt, Int: 1}, Line: 10}
	leafB := &ir.Node{Op: ir.OpLiteral, Value: &ir.Literal{Kind: ir.LitInt, Int: 2}, Line: 11}
	arg := &ir.Node{Op: ir.OpCall, Kids: []*ir.Node{nil}, Args: []*ir.Arg{{Value: leafA}, {Value: leafB}}}
	fn := &ir.Func{Body: []*ir.Node{arg}, Params: []*ir.Param{{Default: leafB}}}
	u := &ir.Unit{Profile: testProfile, Main: &ir.Func{}, Functions: []*ir.Func{fn}}
	p, err := Build(context.Background(), u, Options{MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if p.FunctionCount() != 2 {
		t.Fatalf("function count %d", p.FunctionCount())
	}
	_, roots, ok := p.Function(1)
	if !ok || len(roots) != 1 {
		t.Fatalf("function roots %v %t", roots, ok)
	}
	call := roots[0]
	if p.Child(call, 0) != None || p.Argument(call, 0) == None || p.Argument(call, 1) == None || p.Argument(call, 0) == p.Argument(call, 1) {
		t.Fatal("children/arguments order or nil slot incorrect")
	}
	def := p.Default(1, 0)
	defaultValue, ok := p.Value(def)
	if !ok || defaultValue.HIR != leafB || def == p.Argument(call, 1) {
		t.Fatal("parameter default must have its own occurrence pointing to the default HIR")
	}
	if p.Child(call, 1) != None || p.Argument(call, -1) != None {
		t.Fatal("out of range lookup should be None")
	}
	if _, _, ok := p.Function(-1); ok {
		t.Fatal("negative function index accepted")
	}
}

func TestSourcesUnionFoldChildrenAndConstantBudget(t *testing.T) {
	u, _ := lowerSource(t, "<?php\n'a'.'b';", 64)
	root := u.Main.Body[0].Kids[0]
	left, right := root.Kids[0], root.Kids[1]
	sources := map[*ir.Node]Sources{root: 1, left: 2, right: 4}
	p, err := Build(context.Background(), u, Options{Passes: FoldConstants, MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 2, Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	v := valuesForHIR(p, root)[0]
	if !v.IsConstant || v.Constant.Str != "ab" || p.Child(p.functions[0].roots[0], 0) == None {
		t.Fatal("fold lost result or child edges")
	}
	if v.Sources != 7 || v.HIR.Line != root.Line {
		t.Fatalf("provenance/line lost: %#v", v)
	}
	if len(sources) != 3 {
		t.Fatal("Build modified caller source map")
	}
	u2, _ := lowerSource(t, "<?php 'a'.'b'; 'c'.'d';", 64)
	p2, err := Build(context.Background(), u2, Options{Passes: FoldConstants, MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	folds := 0
	for id := ID(1); uint64(id) <= uint64(p2.Len()); id++ {
		v, _ := p2.Value(id)
		if v.HIR.Op == ir.OpBinary && v.IsConstant && v.Type == String {
			folds++
		}
	}
	if folds != 1 {
		t.Fatalf("whole-program folded string byte budget exceeded: %d folded", folds)
	}
}

func TestBuildRejectsInvalidInputsAndLimitsWithoutPartialProgram(t *testing.T) {
	u, _ := lowerSource(t, "<?php 1;", 64)
	valid := Options{MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 10}
	if p, err := Build(nil, u, valid); err == nil || p != nil {
		t.Fatalf("nil ctx => %v, %v", p, err)
	}
	if p, err := Build(context.Background(), nil, valid); err == nil || p != nil {
		t.Fatalf("nil unit => %v, %v", p, err)
	}
	if p, err := Build(context.Background(), &ir.Unit{Profile: testProfile}, valid); err == nil || p != nil {
		t.Fatalf("nil main => %v, %v", p, err)
	}
	for _, bad := range []Options{{MaxNodes: 0, MaxDepth: 1}, {MaxNodes: 1, MaxDepth: 1025}, {MaxNodes: 1, MaxDepth: 1, MaxConstantBytes: -1}, {MaxNodes: 1, MaxDepth: 1, Passes: 0x80}} {
		if p, err := Build(context.Background(), u, bad); err == nil || p != nil {
			t.Errorf("invalid options accepted: %#v", bad)
		}
	}
	badProfile := *u
	badProfile.Profile.IntWidth = 16
	if p, err := Build(context.Background(), &badProfile, valid); err == nil || p != nil {
		t.Fatal("invalid profile accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if p, err := Build(canceled, u, valid); !errors.Is(err, context.Canceled) || p != nil {
		t.Fatalf("canceled ctx => %v, %v", p, err)
	}
	for _, opts := range []Options{{MaxNodes: 1, MaxDepth: 20}, {MaxNodes: 100, MaxDepth: 1}} {
		p, err := Build(context.Background(), u, opts)
		if !errors.Is(err, ErrLimit) || p != nil {
			t.Errorf("limit must fail atomically: %v, %v", p, err)
		}
	}
	cyclic := &ir.Node{Op: ir.OpUnary, Text: "+"}
	cyclic.Kids = []*ir.Node{cyclic}
	cycleUnit := &ir.Unit{Profile: testProfile, Main: &ir.Func{Body: []*ir.Node{cyclic}}}
	if p, err := Build(context.Background(), cycleUnit, valid); err == nil || p != nil {
		t.Fatalf("cycle => %v, %v", p, err)
	}
}

func TestAllPassesDoNotFoldFlaggedOrEffectfulNodes(t *testing.T) {
	for _, n := range []*ir.Node{
		{Op: ir.OpLiteral, Value: &ir.Literal{Kind: ir.LitInt, Int: 2}, Flags: ir.FlagQuiet},
		{Op: ir.OpBinary, Text: "+", Kids: []*ir.Node{{Op: ir.OpLiteral, Value: &ir.Literal{Kind: ir.LitInt, Int: 1}}, {Op: ir.OpLiteral, Value: &ir.Literal{Kind: ir.LitInt, Int: 2}}}, Effects: ir.MayWarn},
	} {
		u := &ir.Unit{Profile: testProfile, Main: &ir.Func{Body: []*ir.Node{n}}}
		p, err := Build(context.Background(), u, Options{Passes: AllPasses, MaxNodes: 100, MaxDepth: 20, MaxConstantBytes: 100})
		if err != nil {
			t.Fatal(err)
		}
		v := valuesForHIR(p, n)[0]
		if v.IsConstant {
			t.Fatalf("unsafe fold: %#v", v)
		}
	}
}

// A 32-bit profile cannot hold an int outside int32. Such a literal in
// supplied HIR is not proven Int, so it neither folds nor specializes.
func TestOutOfRangeIntLiteralIsNotTypedInt(t *testing.T) {
	u, _ := lowerSource(t, "<?php 1 | 2;", 32)
	var lit *ir.Node
	walkHIR(u.Main.Body[0], func(n *ir.Node) {
		if n.Op == ir.OpLiteral && n.Value.Kind == ir.LitInt && n.Value.Int == 1 {
			lit = n
		}
	})
	lit.Value = &ir.Literal{Kind: ir.LitInt, Int: 1 << 40}
	p, err := Build(context.Background(), u, Options{Passes: AllPasses, MaxNodes: 10000, MaxDepth: 128, MaxConstantBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
		v, _ := p.Value(id)
		switch {
		case v.HIR == lit && (v.Type == Int || v.IsConstant):
			t.Errorf("literal 1<<40: type %v, constant %t", v.Type, v.IsConstant)
		case v.HIR.Op == ir.OpBinary && (v.Specialization != Generic || v.IsConstant):
			t.Errorf("1<<40 | 2: specialization %v, constant %t", v.Specialization, v.IsConstant)
		}
	}
}
