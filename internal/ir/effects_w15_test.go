package ir

import (
	"math"
	"testing"
)

// REQ-024/054; T-047/048/107/108. These must be conservative even when
// the value is discarded. PHP warnings can invoke a throwing user handler.
func TestW15EffectsKeepDiagnostics(t *testing.T) {
	nan := &Node{Op: OpLiteral, Value: &Literal{Kind: LitFloat, Float: math.NaN()}}
	zero := &Node{Op: OpLiteral, Value: &Literal{Kind: LitInt}}
	minusOne := &Node{Op: OpLiteral, Value: &Literal{Kind: LitInt, Int: -1}}
	for _, n := range []*Node{
		{Op: OpBinary, Text: "**", Kids: []*Node{zero, minusOne}},
		{Op: OpUnary, Text: "!", Kids: []*Node{nan}},
		{Op: OpUnary, Text: "!", Kids: []*Node{{Op: OpVar}}},
		{Op: OpCast, Text: "bool", Kids: []*Node{nan}},
		{Op: OpCast, Text: "string", Kids: []*Node{nan}},
		{Op: OpInterp, Kids: []*Node{nan}},
		{Op: OpBinary, Text: ".", Kids: []*Node{nan, zero}},
		{Op: OpEcho, Kids: []*Node{nan}},
		{Op: OpLogical}, {Op: OpEmpty}, {Op: OpTernary},
		{Op: OpIf}, {Op: OpWhile}, {Op: OpDo}, {Op: OpFor},
	} {
		if e := ownEffects(n); e&MayWarn == 0 {
			t.Errorf("%s %s: missing diagnostic effect: %s", n.Op, n.Text, e)
		}
	}
	if e := ownEffects(&Node{Op: "future-operation"}); e&Anything != Anything {
		t.Fatalf("unknown operation is not conservative: %s", e)
	}
}

func TestOperationEffectsMalformedClosure(t *testing.T) {
	n := &Node{Op: OpClosure, Func: &Func{Captures: []*Capture{nil}}}
	if e := OperationEffects(n); e != AllEffects {
		t.Fatalf("malformed closure effects: %s", e)
	}
}
