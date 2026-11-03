package runtime

// This deliberately limited test adapter replays SSA with the existing W-10,
// W-12 and W-14 runtime. It is not a production interpreter or a VM acceptance.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/ir/ssa"
	"github.com/HazelnutParadise/phego/internal/profile"
)

type w15Case struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Result      string   `json:"result"`
	Error       []string `json:"error"`
	Diagnostics [][3]any `json:"diagnostics"`
	Output      string   `json:"output"`
}

type w15Replay struct {
	p           *ssa.Program
	x           *execution
	slots       map[string]*value
	temps       []value
	seen        int
	diagnostics [][3]any
	returned    string
	before      func(*ir.Node) error
}

func w15Build(t testing.TB, source string, width int, passes ssa.Passes) *ssa.Program {
	t.Helper()
	prof, err := profile.Resolve("8.5.12", width, false)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("<?php " + source)
	tree, err := frontend.Parse(src, frontend.LexOptions{IntWidth: width, MaxBytes: len(src) + 1})
	if err != nil {
		t.Fatal(err)
	}
	u, err := ir.Lower(tree, prof, ir.Options{File: "w15.php"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := ssa.Build(context.Background(), u, ssa.Options{Passes: passes, MaxNodes: 100000, MaxDepth: 256, MaxConstantBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func w15Literal(l ir.Literal) value {
	switch l.Kind {
	case ir.LitNull:
		return nullValue()
	case ir.LitBool:
		return boolValue(l.Bool)
	case ir.LitInt:
		return value{kind: valueInt, integer: l.Int}
	case ir.LitFloat:
		return floatValue(l.Float)
	case ir.LitString:
		return bytesValue([]byte(l.Str))
	}
	panic("invalid test literal")
}

func (r *w15Replay) slot(name string) (*value, error) {
	if p := r.slots[name]; p != nil {
		return p, nil
	}
	p := new(value)
	if err := r.x.local(p); err != nil {
		return nil, err
	}
	r.slots[name] = p
	return p, nil
}

func (r *w15Replay) location(id ssa.ID) (*value, error) {
	v, ok := r.p.Value(id)
	if !ok {
		return nil, fmt.Errorf("missing location")
	}
	n := v.HIR
	switch n.Op {
	case ir.OpVar:
		return r.slot(n.Var.Name)
	case ir.OpVarVar:
		v, err := r.eval(r.p.Child(id, 0))
		if err != nil {
			return nil, err
		}
		name, err := r.x.heap.toString(v)
		if err != nil {
			return nil, err
		}
		return r.slot(name)
	case ir.OpDim:
		p, err := r.location(r.p.Child(id, 0))
		if err != nil {
			return nil, err
		}
		key, err := r.eval(r.p.Child(id, 1))
		if err != nil {
			return nil, err
		}
		return r.x.heap.element(p, key)
	}
	return nil, fmt.Errorf("unsupported test location %s", n.Op)
}

func (r *w15Replay) temporary(v value) value { retain(v); r.temps = append(r.temps, v); return v }
func (r *w15Replay) clearTemps() error {
	var result error
	for len(r.temps) > 0 {
		i := len(r.temps) - 1
		v := r.temps[i]
		r.temps = r.temps[:i]
		result = r.x.heap.mergeObjectErrors(result, r.x.heap.release(v))
	}
	return result
}

func (r *w15Replay) eval(id ssa.ID) (out value, err error) {
	if id == ssa.None {
		return nullValue(), nil
	}
	v, ok := r.p.Value(id)
	if !ok {
		return value{}, fmt.Errorf("missing SSA value")
	}
	n := v.HIR
	if c := r.x.check(); c.kind != endNormal {
		return value{}, c
	}
	if r.before != nil {
		if err := r.before(n); err != nil {
			return value{}, err
		}
	}
	defer func() {
		for r.seen < len(r.x.heap.diagnostics) {
			d := r.x.heap.diagnostics[r.seen]
			r.seen++
			r.diagnostics = append(r.diagnostics, [3]any{float64(d.level), d.message, float64(n.Line)})
		}
	}()
	if v.IsConstant {
		return w15Literal(v.Constant), nil
	}
	h := r.x.heap
	child := func(i int) (value, error) { return r.eval(r.p.Child(id, i)) }
	switch n.Op {
	case ir.OpLiteral:
		return w15Literal(*n.Value), nil
	case ir.OpConst:
		if n.Name.Resolved == "NAN" {
			return floatValue(math.NaN()), nil
		}
	case ir.OpVar, ir.OpVarVar:
		p, err := r.location(id)
		if err != nil {
			return value{}, err
		}
		if p.kind == valueUndefined && n.Flags&ir.FlagQuiet == 0 {
			h.warn(levelWarning, "Undefined variable $"+n.Var.Name)
		}
		return p.deref(), nil
	case ir.OpDim:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		key, err := child(1)
		if err != nil {
			return value{}, err
		}
		k, err := h.arrayKey(key, false)
		if err != nil {
			return value{}, err
		}
		if a = a.deref(); a.kind == valueArray {
			if b, ok := a.array.lookup(k); ok {
				return b.val.deref(), nil
			}
		}
		return value{}, fmt.Errorf("unsupported missing test element")
	case ir.OpAssign:
		p, err := r.location(r.p.Child(id, 0))
		if err != nil {
			return value{}, err
		}
		a, err := child(1)
		if err != nil {
			return value{}, err
		}
		return a, h.assign(p, a)
	case ir.OpAssignRef:
		p, err := r.location(r.p.Child(id, 0))
		if err != nil {
			return value{}, err
		}
		q, err := r.location(r.p.Child(id, 1))
		if err != nil {
			return value{}, err
		}
		return q.deref(), h.bind(p, q)
	case ir.OpUnset:
		for i := range n.Kids {
			p, err := r.location(r.p.Child(id, i))
			if err != nil {
				return value{}, err
			}
			if err = h.unset(p); err != nil {
				return value{}, err
			}
		}
		return nullValue(), nil
	case ir.OpArray:
		a := r.temporary(h.newArrayValue())
		for i := range n.Kids {
			itemID := r.p.Child(id, i)
			item, _ := r.p.Value(itemID)
			var slot *value
			if r.p.Child(itemID, 0) == ssa.None {
				slot, err = h.appendElement(&a)
			} else {
				key, e := r.eval(r.p.Child(itemID, 0))
				if e != nil {
					return value{}, e
				}
				slot, err = h.element(&a, key)
			}
			if err != nil {
				return value{}, err
			}
			if item.HIR.Flags&ir.FlagRef != 0 {
				q, e := r.location(r.p.Child(itemID, 1))
				if e != nil {
					return value{}, e
				}
				err = h.bind(slot, q)
			} else {
				x, e := r.eval(r.p.Child(itemID, 1))
				if e != nil {
					return value{}, e
				}
				err = h.assign(slot, x)
			}
			if err != nil {
				return value{}, err
			}
		}
		return a, nil
	case ir.OpBinary:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		b, err := child(1)
		if err != nil {
			return value{}, err
		}
		if v.Specialization == ssa.IntBitwise && (a.kind != valueInt || b.kind != valueInt) {
			return value{}, fmt.Errorf("invalid integer proof")
		}
		if n.Text == "!==" {
			eq, e := identical(a, b)
			return boolValue(!eq), e
		}
		return php8512Binary(h, n.Text, a, b)
	case ir.OpUnary:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		if v.Specialization == ssa.BoolNot {
			if a.kind != valueBool {
				return value{}, fmt.Errorf("invalid boolean proof")
			}
			return boolValue(!a.boolean), nil
		}
		op := n.Text
		if op == "+" {
			op = "pos"
		}
		if op == "-" {
			op = "neg"
		}
		return php8512Unary(h, op, a)
	case ir.OpCast:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		return php8512Unary(h, "("+n.Text+")", a)
	case ir.OpLogical:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		yes := h.toBool(a)
		if n.Text == "&&" && !yes || n.Text == "||" && yes {
			return boolValue(yes), nil
		}
		b, err := child(1)
		if err != nil {
			return value{}, err
		}
		other := h.toBool(b)
		if n.Text == "xor" {
			other = yes != other
		}
		return boolValue(other), nil
	case ir.OpTernary:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		if h.toBool(a) {
			if r.p.Child(id, 1) == ssa.None {
				return a, nil
			}
			return child(1)
		}
		return child(2)
	case ir.OpCoalesce:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		if a.kind != valueNull && a.kind != valueUndefined {
			return a, nil
		}
		return child(1)
	case ir.OpIsset:
		for i := range n.Kids {
			a, err := child(i)
			if err != nil {
				return value{}, err
			}
			if a.kind == valueNull || a.kind == valueUndefined {
				return boolValue(false), nil
			}
		}
		return boolValue(true), nil
	case ir.OpNew:
		if n.Name == nil || n.Name.Resolved != "W15Probe" {
			break
		}
		a, err := r.eval(r.p.Argument(id, 0))
		if err != nil {
			return value{}, err
		}
		fail := false
		if len(n.Args) > 1 {
			b, e := r.eval(r.p.Argument(id, 1))
			if e != nil {
				return value{}, e
			}
			fail = h.toBool(b)
		}
		name := a.text
		obj, err := r.x.object("W15Probe", func(x *execution) completion {
			if c := x.write([]byte("D:" + name + "\n")); c.kind != endNormal {
				return c
			}
			if fail {
				return completion{kind: endThrow, err: &Throwable{"RuntimeException", "destruct:" + name}}
			}
			return completion{}
		})
		if err != nil {
			return value{}, err
		}
		return r.temporary(obj), nil
	case ir.OpCall:
		args := make([]value, len(n.Args))
		for i := range args {
			args[i], err = r.eval(r.p.Argument(id, i))
			if err != nil {
				return value{}, err
			}
		}
		if n.Name == nil {
			break
		}
		switch n.Name.Resolved {
		case "w15mark":
			c := r.x.write([]byte("C:" + args[0].text + "\n"))
			if c.kind != endNormal {
				return value{}, c
			}
			return h.number(5), nil
		case "extract":
			pos := 0
			for e, ok := args[0].array.next(&pos); ok; e, ok = args[0].array.next(&pos) {
				p, err := r.slot(e.key.text)
				if err != nil {
					return value{}, err
				}
				if err = h.assign(p, e.val); err != nil {
					return value{}, err
				}
			}
			return h.number(int64(pos)), nil
		case "compact":
			a := r.temporary(h.newArrayValue())
			for _, name := range args {
				p, err := r.slot(name.text)
				if err != nil {
					return value{}, err
				}
				if err = h.setElement(&a, name, *p); err != nil {
					return value{}, err
				}
			}
			return a, nil
		}
	case ir.OpExpr, ir.OpVoid:
		_, err := child(0)
		cleanup := r.clearTemps()
		return nullValue(), h.mergeObjectErrors(err, cleanup)
	case ir.OpReturn:
		a, err := child(0)
		if err != nil {
			return value{}, err
		}
		r.returned = encodeValue(a)
		c := ownReturn(completion{kind: endReturn, value: a})
		if err := r.clearTemps(); err != nil {
			return value{}, r.x.replaceOwnedCompletion(c, r.completion(err))
		}
		return value{}, c
	case ir.OpBlock:
		for i := range n.Kids {
			if _, err := child(i); err != nil {
				return value{}, err
			}
		}
		return nullValue(), nil
	case ir.OpIf:
		for i := 0; i < len(n.Kids); i += 2 {
			if i == len(n.Kids)-1 {
				return child(i)
			}
			a, err := child(i)
			if err != nil {
				return value{}, err
			}
			if h.toBool(a) {
				return child(i + 1)
			}
		}
		return nullValue(), nil
	case "policy_check", "quota_charge", "cancel_check":
		return nullValue(), nil
	}
	return value{}, fmt.Errorf("unsupported W15 test operation %s %s", n.Op, n.Text)
}

func (r *w15Replay) completion(err error) completion {
	var c completion
	if errors.As(err, &c) {
		return c
	}
	var th *Throwable
	if errors.As(err, &th) {
		return completion{kind: endThrow, err: th}
	}
	return completion{kind: endInternal, err: err}
}

func w15Run(t *testing.T, p *ssa.Program, before func(*ir.Node) error) w15Case {
	t.Helper()
	x := newTestExecution(t, readyExecutionRequest(t), p.Profile().IntWidth, nil, executionLimits{depth: 64, callbacks: 10000, output: 1 << 20})
	r := w15Replay{p: p, x: x, slots: map[string]*value{}, returned: "N", before: before}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, roots, _ := p.Function(0)
	result := x.run(ctx, func(*execution) completion {
		for _, id := range roots {
			if _, err := r.eval(id); err != nil {
				return r.completion(err)
			}
		}
		return completion{}
	})
	if !result.cleanupComplete || result.cleanupErr != nil {
		t.Fatalf("cleanup: %+v", result)
	}
	out := w15Case{Result: r.returned, Diagnostics: r.diagnostics, Output: string(result.output)}
	if result.end.kind != endNormal && result.end.kind != endReturn {
		out.Result = ""
		var th *Throwable
		if errors.As(result.end.err, &th) {
			out.Error = []string{th.Class, th.Message}
		} else {
			out.Error = []string{"host", fmt.Sprint(result.end.err)}
		}
	}
	return out
}

func TestW15PinnedDifferential(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			dir := os.Getenv("PHEGO_W15_REFERENCE")
			if dir == "" {
				dir = "../../tests/optimization/w15"
			}
			raw, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("ref-%d.json", width)))
			if err != nil {
				t.Fatal(err)
			}
			var ref struct {
				PHP     string    `json:"php"`
				IntSize int       `json:"int_size"`
				Cases   []w15Case `json:"cases"`
			}
			if err = json.Unmarshal(raw, &ref); err != nil {
				t.Fatal(err)
			}
			if ref.PHP != "8.5.12" || ref.IntSize*8 != width || len(ref.Cases) != 285 {
				t.Fatal("wrong or incomplete reference")
			}
			for _, c := range ref.Cases {
				t.Run(c.Name, func(t *testing.T) {
					for _, passes := range []ssa.Passes{0, ssa.FoldConstants, ssa.SpecializeTypes, ssa.AllPasses} {
						p := w15Build(t, c.Source, width, passes)
						got := w15Run(t, p, nil)
						if got.Result != c.Result || got.Output != c.Output || !reflect.DeepEqual(append([]string{}, got.Error...), append([]string{}, c.Error...)) || !reflect.DeepEqual(append([][3]any{}, got.Diagnostics...), append([][3]any{}, c.Diagnostics...)) {
							t.Fatalf("passes=%d\nPHP: %+v\nGo:  %+v", passes, c, got)
						}
					}
				})
			}
		})
	}
}
