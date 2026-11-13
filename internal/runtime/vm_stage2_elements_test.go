package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 2 review fixes: REQ-025, REQ-026; T-049, T-050. An element
// location stays valid only within the instructions the compiler emits after
// its FetchDim. Hand-built images that kept one past a write to the same
// array would make the write land in a bucket another value shares, or
// leave an undefined bucket behind; the verifier rejects them, so they
// never reach the VM.
var vmStaleElementImages = map[string]func(m *vmFn){
	// $a = [[1]]; e = &$a[0]; $b = $a; e = "x": both arrays would change.
	"write after the array is copied": func(m *vmFn) {
		m.store("a", m.arrayOf(m.arrayOf(m.k(1))))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		m.store("b", m.load("a"))
		m.emit(bytecode.OpStore, 0, e, m.k("x"), 0)
		m.ret(m.arrayOf(m.load("a"), m.load("b")))
	},
	// e = &$a[0]; then the copy shares the array before the write.
	"write after a load of the base": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k(1), m.k(2)))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		copied := m.load("a")
		m.emit(bytecode.OpStore, 0, e, m.k("x"), 0)
		m.ret(m.arrayOf(m.load("a"), copied))
	},
	// e = &$a[0]; foreach ($a as $v) { e = "x"; echo $v; }: the loop would
	// see the write.
	"write during a foreach over the array": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k("p"), m.k("q")))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(1))
		m.foreachLoop(m.load("a"), "", "v", func() {
			m.emit(bytecode.OpStore, 0, e, m.k("x"), 0)
			m.echo(m.load("v"))
		})
		m.ret(m.load("a"))
	},
	// e = &$a[0]; unset(e): the bucket would be left undefined, and the
	// return would end as an internal error.
	"unset of the element": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k(1)))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		m.emit(bytecode.OpUnset, 0, e, 0, 0)
		m.ret(m.load("a"))
	},
	"isset of the element": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k(1)))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		r := m.reg(bytecode.Value)
		m.emit(bytecode.OpIsset, 0, r, e, 0)
		m.ret(r)
	},
	// $a[0][1] = $a, the value read by ArraySet itself after the fetch:
	// the array would be stored into its own element.
	"the base as the value of the write": func(m *vmFn) {
		m.store("a", m.arrayOf(m.arrayOf(m.k(1))))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		m.emit(bytecode.OpArraySet, uint16(bytecode.ValueLocation), e, m.k(1), m.loc("a"))
		m.ret(m.load("a"))
	},
	"the element moved to another register": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k(1)))
		e, other := m.elem(), m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		m.emit(bytecode.OpMove, 0, other, e, 0)
		m.emit(bytecode.OpStore, 0, other, m.k("x"), 0)
		m.ret(m.load("a"))
	},
	"a use after the write that ended the sequence": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k(1)))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		m.emit(bytecode.OpStore, 0, e, m.k("x"), 0)
		m.emit(bytecode.OpStore, 0, e, m.k("y"), 0)
		m.ret(m.load("a"))
	},
	// The element is defined on every path, but the loop's back edge
	// lands between the fetch and its use.
	"a use reached by a jump": func(m *vmFn) {
		m.store("a", m.arrayOf(m.k(1)))
		e := m.elem()
		m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
		head := m.pc()
		m.emit(bytecode.OpStore, 0, e, m.k("x"), 0)
		br := m.branch(m.k(false))
		m.patch(br, head, m.pc())
		m.ret(m.load("a"))
	},
}

func TestVMStaleElementLocationsAreRejected(t *testing.T) {
	for name, build := range vmStaleElementImages {
		t.Run(name, func(t *testing.T) {
			a := newAsm(t, 64)
			build(a.fn("{main}", 0, 2, "a", "b", "v"))
			_, err := bytecode.Build(context.Background(), a.image(), bytecode.Options{Limits: bytecode.DefaultLimits()})
			var be *bytecode.Error
			if !errors.As(err, &be) || be.Kind != bytecode.ErrInvalid || !strings.Contains(be.Detail, "element location") || be.PC < 0 {
				t.Fatalf("Build = %v, want a located ErrInvalid for the element location", err)
			}
		})
	}
}
