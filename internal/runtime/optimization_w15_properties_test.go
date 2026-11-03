package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/ir/ssa"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// REQ-024/034/054; T-047/048/067/068/107/108. Explicit guard operations are
// formal compiler-contract tests; real VM policy wiring remains W-17/W-21.
func TestW15GuardsStayBeforeEffects(t *testing.T) {
	prof, _ := profile.Resolve("8.5.12", 64, false)
	for _, stop := range []string{"", "policy_check", "quota_charge", "cancel_check"} {
		var baseline []string
		for _, passes := range []ssa.Passes{0, ssa.FoldConstants, ssa.SpecializeTypes, ssa.AllPasses} {
			lit := func(i int64) *ir.Node {
				return &ir.Node{Op: ir.OpLiteral, Line: 1, Value: &ir.Literal{Kind: ir.LitInt, Int: i}}
			}
			u := &ir.Unit{Profile: prof, Main: &ir.Func{Body: []*ir.Node{
				{Op: "policy_check"}, {Op: "quota_charge"}, {Op: "cancel_check"},
				{Op: ir.OpReturn, Kids: []*ir.Node{{Op: ir.OpBinary, Text: "+", Kids: []*ir.Node{lit(1), lit(2)}}}},
			}}}
			p, err := ssa.Build(context.Background(), u, ssa.Options{Passes: passes, MaxNodes: 100, MaxDepth: 32})
			if err != nil {
				t.Fatal(err)
			}
			var events []string
			got := w15Run(t, p, func(n *ir.Node) error {
				if n.Op == "policy_check" || n.Op == "quota_charge" || n.Op == "cancel_check" || n.Op == ir.OpReturn {
					events = append(events, string(n.Op))
				}
				if string(n.Op) == stop {
					return completion{kind: endCancel, err: context.Canceled}
				}
				return nil
			})
			if passes == 0 {
				baseline = events
			} else if !reflect.DeepEqual(events, baseline) {
				t.Fatalf("guard order %d: %v != %v", passes, events, baseline)
			}
			if stop != "" && (len(got.Error) == 0 || events[len(events)-1] != stop) {
				t.Fatalf("guard did not stop: %+v %v", got, events)
			}
		}
	}
}

// Fuzzing compares real runtime scalar actions before and after every pass.
// Native PHP additionally anchors the frozen random corpus in the reference.
func FuzzW15Differential(f *testing.F) {
	for _, pair := range [][2]int64{{0, 0}, {1, -1}, {2147483647, 1}, {-2147483648, -1}, {9223372036854775807, 1}, {-9223372036854775808, -1}} {
		f.Add(pair[0], pair[1], uint8(0))
	}
	f.Fuzz(func(t *testing.T, a, b int64, kind uint8) {
		width := 32
		if kind&1 != 0 {
			width = 64
		}
		op := []string{"+", "-", "*", "&", "|", "^"}[int(kind>>1)%6]
		src := fmt.Sprintf("return ((%d)%s(%d))+(2*3);", a, op, b)
		baseline := w15Run(t, w15Build(t, src, width, 0), nil)
		for _, passes := range []ssa.Passes{ssa.FoldConstants, ssa.SpecializeTypes, ssa.AllPasses} {
			got := w15Run(t, w15Build(t, src, width, passes), nil)
			if !reflect.DeepEqual(got, baseline) {
				t.Fatalf("width=%d passes=%d source=%s\noff=%+v\non=%+v", width, passes, src, baseline, got)
			}
		}
	})
}

func TestW15CompilerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prof, _ := profile.Resolve("8.5.12", 64, false)
	p, err := ssa.Build(ctx, &ir.Unit{Profile: prof, Main: &ir.Func{}}, ssa.Options{MaxNodes: 10, MaxDepth: 10})
	if p != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v %v", p, err)
	}
}
