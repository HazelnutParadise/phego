package runtime

import (
	"context"
	"math/rand/v2"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 2 review fixes: REQ-016, REQ-025; T-009, T-031. String offset
// writes stay linear when they extend the string and observe a host stop
// while they pad it.

// vmOffsetLoop is "$s = init; for ($i = from; $i < n; $i++) $s[$i] = 'x';
// return $s[n-1];", the write PHP 8.5.12 makes in place.
func vmOffsetLoop(t testing.TB, init string, from, n int) *vmAsm {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "s", "i")
	m.store("s", m.k(init))
	m.store("i", m.k(from))
	loop := m.pc()
	c := m.bin(bytecode.Less, m.load("i"), m.k(n))
	br := m.branch(c)
	body := m.pc()
	m.set("s", m.load("i"), m.k("x"))
	m.incdec(bytecode.PreInc, "i")
	m.jump(loop)
	end := m.pc()
	m.patch(br, body, end)
	m.ret(m.get(m.load("s"), m.k(n-1), false))
	return a
}

// A loop that appends one byte at a time costs time linear in the length,
// as PHP 8.5.12's in-place write does.
func TestVMStringOffsetAppendsAreLinear(t *testing.T) {
	for _, n := range []int{1, 2, 3, 1000, 200000} {
		res := runAsm(t, vmOffsetLoop(t, "", 0, n))
		if b, ok := res.Return.([]byte); res.End != Completed || !ok || string(b) != "x" {
			t.Fatalf("n=%d: %v %v %+v", n, res.End, res.Err, res.Return)
		}
	}
	// The bytes the run allocates grow with the string, not with its
	// square: one copy per write would allocate 20 GB here.
	p := vmOffsetLoop(t, "", 0, 200000).program()
	var before, after goruntime.MemStats
	goruntime.ReadMemStats(&before)
	if res := p.Run(vmConfig(t, 64)); res.End != Completed {
		t.Fatal(res.Err)
	}
	goruntime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 64<<20 {
		t.Errorf("200 000 appends allocated %d bytes", got)
	}
}

// The bytes are those of a copy per write, however the writes interleave:
// a write through another variable holding the same string, an overwrite,
// a gap that pads, and a write before the end leave every other holder's
// string as it was.
func TestVMStringOffsetWritesKeepOtherHoldersUnchanged(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "s", "t", "u")
	m.store("s", m.k(""))
	m.set("s", m.k(0), m.k("a"))
	m.set("s", m.k(1), m.k("b"))
	m.store("t", m.load("s"))     // $t = "ab", sharing the buffer
	m.set("s", m.k(2), m.k("c"))  // $s = "abc"
	m.set("t", m.k(2), m.k("Z"))  // $t = "abZ": the buffer is not $t's to extend
	m.set("s", m.k(3), m.k("d"))  // $s = "abcd"
	m.store("u", m.load("s"))     // $u = "abcd"
	m.set("s", m.k(1), m.k("B"))  // $s = "aBcd"
	m.set("u", m.k(6), m.k("e"))  // $u = "abcd  e"
	m.set("s", m.k(-1), m.k("D")) // $s = "aBcD"
	m.set("s", m.k(4), m.k("f"))  // $s = "aBcDf"
	m.ret(m.arrayOf(m.load("s"), m.load("t"), m.load("u")))
	res := runAsm(t, a)
	if res.End != Completed {
		t.Fatalf("%v %v", res.End, res.Err)
	}
	var got []string
	entries, _ := res.Return.([]SnapshotEntry)
	for _, e := range entries {
		b, _ := e.Value.([]byte)
		got = append(got, string(b))
	}
	if want := []string{"aBcDf", "abZ", "abcd  e"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// vmOffsetCopy is the string offset write as one copy of the string, the
// way it was built before offsetString kept its buffer: the reference
// offsetString must match byte for byte.
func vmOffsetCopy(str string, off int64, b byte) string {
	n := int64(len(str))
	var out strings.Builder
	out.Grow(int(max(n, off+1)))
	if off < n {
		out.WriteString(str[:off])
		out.WriteByte(b)
		out.WriteString(str[off+1:])
		return out.String()
	}
	out.WriteString(str)
	out.WriteString(strings.Repeat(" ", int(off-n)))
	out.WriteByte(b)
	return out.String()
}

func vmOffsetState(t testing.TB) *vmState {
	a := newAsm(t, 64)
	a.fn("{main}", 0, 2).retNull()
	cfg := vmConfig(t, 64)
	x, err := newExecution(cfg.Context, cfg.Request, 64, nil, executionLimits{depth: 4, callbacks: 16, output: 1 << 10})
	if err != nil {
		t.Fatal(err)
	}
	return newVMState(a.program(), x, cfg)
}

// Random writes and copies between four variables give the bytes of a copy
// per write: a variable never sees a write made through another one.
func TestVMStringOffsetMatchesACopyPerWrite(t *testing.T) {
	s := vmOffsetState(t)
	never := func() bool { return false }
	rng := rand.New(rand.NewPCG(7, 11))
	got, want := make([]string, 4), make([]string, 4)
	for step := 0; step < 20000; step++ {
		i, j := rng.IntN(4), rng.IntN(4)
		switch rng.IntN(5) {
		case 0:
			got[j], want[j] = got[i], want[i]
		case 1:
			got[i], want[i] = "", ""
		default:
			off := int64(rng.IntN(len(want[i]) + 4))
			b := byte('a' + rng.IntN(26))
			out, ok := s.offsetString(got[i], off, b, never)
			if !ok {
				t.Fatal("stopped without a host stop")
			}
			got[i], want[i] = out, vmOffsetCopy(want[i], off, b)
		}
		for k := range got {
			if got[k] != want[k] {
				t.Fatalf("step %d: variable %d is %q, want %q", step, k, got[k], want[k])
			}
		}
	}
}

// Padding a string far past its end polls for a host stop between chunks:
// a run cancelled while it pads ends without finishing the string.
func TestVMStringOffsetPaddingStopsAfterCancellation(t *testing.T) {
	a := newAsm(t, 64)
	a.fn("{main}", 0, 2).retNull()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := vmConfig(t, 64)
	cfg.Context = ctx
	x, err := newExecution(ctx, cfg.Request, 64, nil, executionLimits{depth: 4, callbacks: 16, output: 1 << 10})
	if err != nil {
		t.Fatal(err)
	}
	s := newVMState(a.program(), x, cfg)
	polls := 0
	const off = 10*vmCopyChunk + 5
	if _, ok := s.offsetString("abc", off, 'x', func() bool { polls++; return polls == 3 }); ok {
		t.Fatal("the string was finished after the host stopped the run")
	}
	if polls != 3 {
		t.Fatalf("%d polls before the stop, want 3", polls)
	}
	// Uncancelled, the same write polls once per chunk and gives PHP's bytes.
	polls = 0
	out, ok := s.offsetString("abc", off, 'x', func() bool { polls++; return false })
	if !ok || len(out) != off+1 || out[:3] != "abc" || out[off] != 'x' || strings.Trim(out[3:off], " ") != "" {
		t.Fatalf("padded string wrong: ok %v, %d bytes", ok, len(out))
	}
	if polls < off/vmCopyChunk {
		t.Fatalf("%d polls for %d bytes", polls, off)
	}
}

func BenchmarkVMStringOffsetAppend(b *testing.B) {
	p := vmOffsetLoop(b, "", 0, 200000).program()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if res := p.Run(vmConfig(b, 64)); res.End != Completed {
			b.Fatal(res.Err)
		}
	}
}

// The overwrite of a byte inside the string still copies it, a known gap;
// this measures that remaining cost.
func BenchmarkVMStringOffsetOverwrite(b *testing.B) {
	p := vmOffsetLoop(b, strings.Repeat("y", 200000), 0, 200000).program()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if res := p.Run(vmConfig(b, 64)); res.End != Completed {
			b.Fatal(res.Err)
		}
	}
}

// The overwrite path against the copy it replaced, on the helper alone.
func BenchmarkVMStringOffsetOverwriteHelper(b *testing.B) {
	str := strings.Repeat("y", 20000)
	b.Run("copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			str = vmOffsetCopy(str, int64(i%20000), 'x')
		}
	})
	s := vmOffsetState(b)
	never := func() bool { return false }
	b.Run("offsetString", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			str, _ = s.offsetString(str, int64(i%20000), 'x', never)
		}
	})
}
