package bytecode

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"testing"
)

// straightLine returns one function of n instructions over a narrow frame.
func straightLine(n, registers int) Image {
	i := validImage()
	f := &i.Functions[0]
	f.Registers = make([]RegisterKind, registers)
	for r := range f.Registers {
		f.Registers[r] = Value
	}
	f.Code = make([]Instruction, n)
	for pc := 0; pc < n-1; pc++ {
		f.Code[pc] = Instruction{Op: OpConst, A: uint32(pc % registers)}
	}
	f.Code[n-1] = Instruction{Op: OpReturn}
	f.Positions = make([]Position, n)
	return i
}

// The default budgets admit an image that reaches MaxInstructions with
// straight-line code over a 64-register frame, through Build and Load.
func TestDefaultLimitsAdmitLargestStraightLineImage(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 1,048,576-instruction image")
	}
	img := straightLine(int(DefaultLimits().MaxInstructions), 64)
	p, err := Build(context.Background(), img, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := Load(context.Background(), data, LoadOptions{Options: testOptions(), Expected: img.Identity, TrustedDigest: sha256.Sum256(data)}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	img.Functions[0].Code = append(img.Functions[0].Code, Instruction{Op: OpReturn})
	img.Functions[0].Positions = append(img.Functions[0].Positions, Position{})
	if _, err := Build(context.Background(), img, testOptions()); !errors.Is(err, ErrLimit) {
		t.Fatalf("one instruction past MaxInstructions: %v", err)
	}
}

// finallyBlocks repeats the try/finally of validImageWithHandler count times.
func finallyBlocks(count int) Image {
	i := validImage()
	f := &i.Functions[0]
	f.Code = nil
	for b := 0; b < count; b++ {
		base := uint32(4 * b)
		f.Code = append(f.Code, Instruction{Op: OpConst}, Instruction{Op: OpLeave, A: base + 4}, Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally})
		f.Handlers = append(f.Handlers, Handler{Start: base, End: base + 2, Target: base + 2, BodyEnd: base + 4, Kind: Finally})
	}
	f.Code = append(f.Code, Instruction{Op: OpConst}, Instruction{Op: OpReturn})
	f.Positions = make([]Position, len(f.Code))
	return i
}

// Handler checks cost O(instructions + handlers log handlers), not one scan
// of every handler per edge, end-finally or earlier handler.
func TestHandlerChecksDoNotScanEveryHandler(t *testing.T) {
	opts := testOptions()
	opts.Limits.MaxWork = 1 << 20
	if _, err := Build(context.Background(), finallyBlocks(4096), opts); err != nil {
		t.Fatalf("4096 sibling finally blocks within %d work: %v", opts.Limits.MaxWork, err)
	}
}

// pairwiseNesting and scanEdge are the former O(h²) and O(h)-per-edge checks,
// kept as the reference for the indexed versions.
func pairwiseNesting(f *Function) bool {
	contains := func(a, z, b, y uint32) bool { return a <= b && y <= z }
	disjoint := func(a, z, b, y uint32) bool { return z <= b || y <= a }
	nested := func(outer, inner Handler) bool {
		return (contains(outer.Start, outer.End, inner.Start, inner.End) && contains(outer.Start, outer.End, inner.Target, inner.BodyEnd)) ||
			(contains(outer.Target, outer.BodyEnd, inner.Start, inner.End) && contains(outer.Target, outer.BodyEnd, inner.Target, inner.BodyEnd))
	}
	for i, h := range f.Handlers {
		for _, p := range f.Handlers[:i] {
			siblings := disjoint(h.Start, h.End, p.Start, p.End) && disjoint(h.Start, h.End, p.Target, p.BodyEnd) &&
				disjoint(h.Target, h.BodyEnd, p.Start, p.End) && disjoint(h.Target, h.BodyEnd, p.Target, p.BodyEnd)
			if !siblings && !nested(p, h) && !nested(h, p) {
				return false
			}
		}
	}
	return true
}

func scanEdge(f *Function, from, to uint32, leave bool) (violations int, first string) {
	for _, h := range f.Handlers {
		fromBody, toBody := from >= h.Target && from < h.BodyEnd, to >= h.Target && to < h.BodyEnd
		fromProtected, toProtected := from >= h.Start && from < h.End, to >= h.Start && to < h.End
		var msg string
		switch {
		case toBody && !fromBody:
			msg = "normal control enters handler body"
		case fromBody && !toBody && !leave:
			msg = "normal control leaves handler body"
		case !fromProtected && toProtected && to != h.Start:
			msg = "control enters protected range after start"
		case fromProtected && !toProtected && h.Kind == Finally && !leave:
			msg = "control leaves finally range without leave"
		}
		if msg != "" {
			if violations == 0 {
				first = msg
			}
			violations++
		}
	}
	return violations, first
}

func TestIndexedHandlerChecksMatchPairwiseReference(t *testing.T) {
	rng := uint64(0x9e3779b97f4a7c15)
	next := func(n int) uint32 {
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		return uint32(rng % uint64(n))
	}
	b, err := newBudget(context.Background(), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for round := 0; round < 200000; round++ {
		n := 2 + int(next(11))
		f := &Function{Code: make([]Instruction, n), Registers: []RegisterKind{Value}}
		for k := int(next(5)); k > 0; k-- {
			h := Handler{Start: next(n), Target: 1 + next(n-1), Kind: Catch + HandlerKind(next(2))}
			h.End = h.Start + 1 + next(n-int(h.Start))
			h.BodyEnd = h.Target + 1 + next(n-int(h.Target))
			if h.Target < h.End && h.BodyEnd > h.Start {
				continue
			}
			f.Handlers = append(f.Handlers, h)
		}
		slices.SortFunc(f.Handlers, func(x, y Handler) int { return cmp.Or(cmp.Compare(x.Start, y.Start), cmp.Compare(y.End, x.End)) })
		if verifyHandlers(b, f) != nil && func() bool {
			for i := 1; i < len(f.Handlers); i++ {
				if f.Handlers[i].Start == f.Handlers[i-1].Start && f.Handlers[i].End == f.Handlers[i-1].End {
					return true
				}
			}
			return false
		}() {
			continue // noncanonical duplicate ranges are rejected before nesting
		}
		want := pairwiseNesting(f)
		if got := verifyHandlerNesting(b, f) == nil; got != want {
			t.Fatalf("nesting %+v: indexed %t, pairwise %t", f.Handlers, got, want)
		}
		if !want {
			continue
		}
		accepted++
		x, err := newHandlerIndex(b, f)
		if err != nil {
			t.Fatal(err)
		}
		for from := uint32(0); from < uint32(n); from++ {
			for to := uint32(0); to < uint32(n); to++ {
				for _, leave := range []bool{false, true} {
					count, msg := scanEdge(f, from, to, leave)
					err := x.checkEdge(from, to, leave)
					if (err == nil) != (count == 0) || count == 1 && err.Error() != invalid(msg).Error() {
						t.Fatalf("edge %d->%d leave=%t in %+v: indexed %v, reference %d %q", from, to, leave, f.Handlers, err, count, msg)
					}
				}
			}
			kind, ok := x.innermostBody(int(from))
			wantKind, width := HandlerKind(0), uint32(n)+1
			for _, h := range f.Handlers {
				if from >= h.Target && from < h.BodyEnd && h.BodyEnd-h.Target < width {
					wantKind, width = h.Kind, h.BodyEnd-h.Target
				}
			}
			if ok != (wantKind != 0) || ok && kind != wantKind {
				t.Fatalf("innermost body at %d in %+v: %v %t, want %v", from, f.Handlers, kind, ok, wantKind)
			}
		}
	}
	if accepted < 10000 {
		t.Fatalf("only %d accepted handler sets exercised", accepted)
	}
}

// referenceVerifyHandlers is the former per-handler verifier: each handler's
// own checks, then crossing against every earlier handler. The indexed
// verifier must report the same error for any handler table.
func referenceVerifyHandlers(f *Function) error {
	for i, h := range f.Handlers {
		if h.Start >= h.End || h.End > uint32(len(f.Code)) || h.Target == 0 || h.Target >= h.BodyEnd || h.BodyEnd > uint32(len(f.Code)) || h.StackDepth != 0 ||
			(h.Target < h.End && h.BodyEnd > h.Start) {
			return invalid("invalid handler ranges")
		}
		if h.Kind == Catch {
			if uint64(h.Register) >= uint64(len(f.Registers)) || f.Registers[h.Register] != Value {
				return invalid("invalid catch register")
			}
		} else if h.Kind == Finally {
			if h.Register != 0 {
				return invalid("finally has register")
			}
		} else {
			return invalid("unknown handler kind")
		}
		if i > 0 {
			p := f.Handlers[i-1]
			if h.Start < p.Start || (h.Start == p.Start && h.End >= p.End) {
				return invalid("noncanonical handler order")
			}
		}
		if !pairwiseNesting(&Function{Handlers: f.Handlers[:i+1]}) {
			return invalid("crossing handler regions")
		}
	}
	return nil
}

func TestVerifyHandlersReportsTheFormerFirstError(t *testing.T) {
	rng := uint64(0x2545f4914f6cdd1d)
	next := func(n int) uint32 {
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		return uint32(rng % uint64(n))
	}
	b, err := newBudget(context.Background(), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	// Two crossing handlers followed by one with an unknown kind: the former
	// verifier reported the crossing first.
	f := &Function{Code: make([]Instruction, 8), Registers: []RegisterKind{Value}, Handlers: []Handler{
		{Start: 0, End: 3, Target: 4, BodyEnd: 6, Kind: Catch},
		{Start: 1, End: 5, Target: 6, BodyEnd: 7, Kind: Catch},
		{Start: 2, End: 3, Target: 7, BodyEnd: 8, Kind: 9},
	}}
	if got, want := verifyHandlers(b, f), referenceVerifyHandlers(f); fmt.Sprint(got) != fmt.Sprint(want) || want == nil {
		t.Fatalf("crossing then unknown kind: got %v, want %v", got, want)
	}
	for round := 0; round < 200000; round++ {
		n := 2 + int(next(11))
		f := &Function{Code: make([]Instruction, n), Registers: []RegisterKind{Value, Location}}
		for k := int(next(5)); k > 0; k-- {
			h := Handler{Start: next(n), Target: next(n), Kind: Catch + HandlerKind(next(2)), Register: uint32(next(2))}
			h.End = h.Start + next(n+1-int(h.Start))
			h.BodyEnd = h.Target + next(n+1-int(h.Target))
			if next(16) == 0 {
				h.Kind = HandlerKind(3 + next(3))
			}
			f.Handlers = append(f.Handlers, h)
		}
		if next(4) != 0 {
			slices.SortFunc(f.Handlers, func(x, y Handler) int { return cmp.Or(cmp.Compare(x.Start, y.Start), cmp.Compare(y.End, x.End)) })
		}
		if got, want := verifyHandlers(b, f), referenceVerifyHandlers(f); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%+v: got %v, want %v", f.Handlers, got, want)
		}
	}
}

// MaxMemory counts conservative accounted allocations, so the handler index
// and nesting check reserve at least what they allocate.
func TestHandlerChecksReserveWhatTheyAllocate(t *testing.T) {
	const n, h = 1 << 16, 4096
	f := &Function{Code: make([]Instruction, n), Registers: []RegisterKind{Value}}
	for i := 0; i < h; i++ {
		kind := Catch
		if i%2 == 1 {
			kind = Finally
		}
		start := uint32(8 * i)
		f.Handlers = append(f.Handlers, Handler{Start: start, End: start + 2, Target: start + 3, BodyEnd: start + 5, Kind: kind})
	}
	for _, check := range []struct {
		name string
		run  func(*budget) error
	}{
		{"nesting", func(b *budget) error { return verifyHandlerNesting(b, f) }},
		{"index", func(b *budget) error { _, err := newHandlerIndex(b, f); return err }},
	} {
		b, err := newBudget(context.Background(), DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		if err := check.run(b); err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&after)
		if allocated := after.TotalAlloc - before.TotalAlloc; b.mem < allocated {
			t.Errorf("%s reserved %d bytes, allocated %d", check.name, b.mem, allocated)
		}
	}
}
