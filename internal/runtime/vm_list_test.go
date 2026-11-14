package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 4: REQ-025, REQ-026; T-049, T-050. The format 1.4 operations
// of destructuring as hand-written bytecode sees them: the list read of
// ArrayGet, FetchDim from a reference in a Value register, and what a
// function too large for liveness leaves in registers. The PHP-visible
// outputs are tests/vm/w17/cases/{list,foreach_list}_*.php.

// list is FETCH_LIST_R: the element e of the Value register arr.
func (f *vmFn) list(arr, key uint32) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpArrayGet, bytecode.ArrayGetList, r, arr, key)
	return r
}

// A list read of a container that is not an array reads null: silently for
// null, with "Cannot use T as array" for every other type, a string
// included, which is not a string container here.
func TestListReadOfNonArrays(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1)
	for _, c := range []any{nil, 5, 1.5, true, false, "abc"} {
		r := m.list(m.k(c), m.k(0))
		m.echo(m.cast(bytecode.CastString, m.bin(bytecode.Identical, r, m.k(nil))))
	}
	m.retNull()
	res := runAsm(t, a)
	want := "1" + strings.Repeat("\nWarning: Cannot use int as array in /w/t.php on line 1\n1", 1) +
		"\nWarning: Cannot use float as array in /w/t.php on line 1\n1" +
		"\nWarning: Cannot use bool as array in /w/t.php on line 1\n1" +
		"\nWarning: Cannot use bool as array in /w/t.php on line 1\n1" +
		"\nWarning: Cannot use string as array in /w/t.php on line 1\n1"
	if res.Err != nil || string(res.Output) != want {
		t.Fatalf("output %q, err %v\nwant %q", res.Output, res.Err, want)
	}
}

// A list read of an array behaves as a plain read: the element, or null
// with the undefined-key warning; a reference in the container is read
// through.
func TestListReadOfArrays(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "x", "r")
	m.store("x", m.arrayOf(m.k(7)))
	m.echo(m.list(m.load("x"), m.k(0)))
	m.echo(m.cast(bytecode.CastString, m.bin(bytecode.Identical, m.list(m.load("x"), m.k(1)), m.k(nil))))
	rr := m.reg(bytecode.Value)
	m.emit(bytecode.OpMakeRef, 0, rr, m.loc("x"), 0)
	m.echo(m.list(rr, m.k(0))) // through the reference
	m.retNull()
	res := runAsm(t, a)
	want := "7\nWarning: Undefined array key 1 in /w/t.php on line 1\n17"
	if res.Err != nil || string(res.Output) != want {
		t.Fatalf("output %q, err %v\nwant %q", res.Output, res.Err, want)
	}
}

// FetchDim from a Value register fetches the element of the array the
// reference holds, creating it, and fails closed when the register holds
// anything but a reference: the compiler never emits that, and FETCH_LIST_W's
// notice for it is not modeled.
func TestFetchDimFromAValueRegister(t *testing.T) {
	flags := uint16(bytecode.FetchWrite | bytecode.FetchForRef | bytecode.FetchContainerValue)
	t.Run("reference", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 1, "x", "e")
		withoutLiveness(m)
		m.store("x", m.arrayOf(m.k(1)))
		rr := m.reg(bytecode.Value)
		m.emit(bytecode.OpMakeRef, 0, rr, m.loc("x"), 0)
		el := m.reg(bytecode.Location)
		m.emit(bytecode.OpFetchDim, flags, el, rr, m.k(1))
		r := m.reg(bytecode.Value)
		m.emit(bytecode.OpMakeRef, 0, r, el, 0)
		m.emit(bytecode.OpBind, uint16(bytecode.BindValue), m.loc("e"), r, 0)
		m.emit(bytecode.OpDrop, 0, rr, 0, 0)
		m.store("e", m.k("E"))
		m.echo(m.get(m.load("x"), m.k(1), false))
		m.retNull()
		res := runAsm(t, a)
		if res.Err != nil || string(res.Output) != "E" {
			t.Fatalf("output %q, err %v; want \"E\"", res.Output, res.Err)
		}
	})
	t.Run("not a reference", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 1, "e")
		arr := m.arrayOf(m.k(1))
		el := m.reg(bytecode.Location)
		m.echoK("a")
		m.emit(bytecode.OpFetchDim, flags, el, arr, m.k(0))
		r := m.reg(bytecode.Value)
		m.emit(bytecode.OpMakeRef, 0, r, el, 0)
		m.emit(bytecode.OpBind, uint16(bytecode.BindValue), m.loc("e"), r, 0)
		m.retNull()
		res := runAsm(t, a)
		var ue *UnsupportedError
		if !errors.As(res.Err, &ue) || string(res.Output) != "a" {
			t.Fatalf("output %q, err %v; want \"a\" and an UnsupportedError", res.Output, res.Err)
		}
	})
}

// The Drop the compiler emits after a destructuring assignment by reference
// releases the reference the right-hand side became, so the element's
// reference count is what PHP's is when the statement ends: a later copy of
// the array separates the element instead of sharing it. Without the Drop,
// a function too large for liveness would keep the extra owner.
func TestDroppedListReferenceIsReleased(t *testing.T) {
	run := func(drop bool) string {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 1, "z", "a", "c")
		withoutLiveness(m)
		// $z = ['k' => [1]]; [&$a] = $z['k']; unset($a); $c = $z; $c['k'] = 'K'; echo $z['k'][0];
		inner := m.arrayOf(m.k(1))
		z := m.newArray()
		m.add(z, m.k("k"), inner)
		m.store("z", z)
		el := m.reg(bytecode.Location)
		m.emit(bytecode.OpFetchDim, uint16(bytecode.FetchWrite|bytecode.FetchForRef), el, m.loc("z"), m.k("k"))
		rr := m.reg(bytecode.Value)
		m.emit(bytecode.OpMakeRef, 0, rr, el, 0)
		el2 := m.reg(bytecode.Location)
		m.emit(bytecode.OpFetchDim, uint16(bytecode.FetchWrite|bytecode.FetchForRef|bytecode.FetchContainerValue), el2, rr, m.k(0))
		r := m.reg(bytecode.Value)
		m.emit(bytecode.OpMakeRef, 0, r, el2, 0)
		m.emit(bytecode.OpBind, uint16(bytecode.BindValue), m.loc("a"), r, 0)
		if drop {
			m.emit(bytecode.OpDrop, 0, rr, 0, 0)
		}
		m.unset("a")
		m.store("c", m.load("z"))
		m.set("c", m.k("k"), m.k("K"))
		m.echo(m.get(m.get(m.load("z"), m.k("k"), false), m.k(0), false))
		m.retNull()
		res := runAsm(t, a)
		if res.Err != nil {
			t.Fatalf("err %v", res.Err)
		}
		return string(res.Output)
	}
	if got := run(true); got != "1" {
		t.Fatalf("with Drop: %q, want \"1\" (the element was separated)", got)
	}
	if got := run(false); got != "K" {
		t.Fatalf("without Drop: %q, want \"K\" (the leaked owner keeps the element shared)", got)
	}
}
