package bytecode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Format 1.3 (W-17 stage 3): REQ-025; T-049, T-050. The frame is
// format12Image's: r0 Value (a defined parameter), r1 Value (defined, an
// array), r2 Location (defined), r3 Value (undefined), r4 Location
// (undefined). The body starts at pc 3.

func format13ValidInstructions() map[string]Instruction {
	return map[string]Instruction{
		"bind locations":           {Op: OpBind, A: 2, B: 2},
		"bind value":               {Op: OpBind, Aux: uint16(BindValue), A: 2, B: 1},
		"make ref":                 {Op: OpMakeRef, A: 3, B: 2},
		"iter init by ref":         {Op: OpIterInit, Aux: uint16(IterByRef), A: 3, B: 2, C: format12Next},
		"iter init by ref temp":    {Op: OpIterInit, Aux: uint16(IterByRef), A: 3, B: 1, C: format12Next},
		"iter next by ref":         {Op: OpIterNext, Aux: uint16(IterByRef | IterLocation), A: 1, B: 2, C: format12Next},
		"iter next by ref value":   {Op: OpIterNext, Aux: uint16(IterByRef), A: 1, B: 3, C: format12Next},
		"fetch for ref":            {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef), A: 4, B: 2, C: 0},
		"fetch for ref append":     {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef | FetchAppend), A: 4, B: 2},
		"fetch for ref key a loca": {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef | FetchKeyLocation), A: 4, B: 2, C: 2},
	}
}

func TestFormat13AcceptsValidOperands(t *testing.T) {
	for name, in := range format13ValidInstructions() {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format12Image(in)) })
	}
	t.Run("make ref defines its register", func(t *testing.T) {
		requireValidBuild(t, format12Image(Instruction{Op: OpMakeRef, A: 3, B: 2}, Instruction{Op: OpOutput, A: 3}))
	})
	t.Run("a by-reference foreach loop", func(t *testing.T) {
		// 3 IterInit r3, L2 (by ref), 8; 4 IterNext r3, L2 (by ref), 7;
		// 5 IterKey r0, r3; 6 Jump 4; 7 Drop r3; 8 Return r0.
		requireValidBuild(t, format12Image(
			Instruction{Op: OpIterInit, Aux: uint16(IterByRef), A: 3, B: 2, C: 8},
			Instruction{Op: OpIterNext, Aux: uint16(IterByRef | IterLocation), A: 3, B: 2, C: 7},
			Instruction{Op: OpIterKey, A: 0, B: 3},
			Instruction{Op: OpJump, A: 4},
			Instruction{Op: OpDrop, A: 3},
			Instruction{Op: OpReturn, A: 0},
		))
	})
}

func TestFormat13RejectsInvalidOperands(t *testing.T) {
	cases := map[string]Instruction{
		"bind value dest":                    {Op: OpBind, A: 0, B: 2},
		"bind value source":                  {Op: OpBind, A: 2, B: 0},
		"bind location with value":           {Op: OpBind, Aux: uint16(BindValue), A: 2, B: 2},
		"bind aux high":                      {Op: OpBind, Aux: 2, A: 2, B: 2},
		"bind c":                             {Op: OpBind, A: 2, B: 2, C: 1},
		"bind undefined source":              {Op: OpBind, A: 2, B: 4},
		"bind undefined value":               {Op: OpBind, Aux: uint16(BindValue), A: 2, B: 3},
		"make ref location dest":             {Op: OpMakeRef, A: 2, B: 2},
		"make ref value source":              {Op: OpMakeRef, A: 3, B: 0},
		"make ref aux":                       {Op: OpMakeRef, Aux: 1, A: 3, B: 2},
		"make ref c":                         {Op: OpMakeRef, A: 3, B: 2, C: 1},
		"make ref undefined":                 {Op: OpMakeRef, A: 3, B: 4},
		"iter init aux 3":                    {Op: OpIterInit, Aux: 3, A: 3, B: 2, C: format12Next},
		"iter init location by val":          {Op: OpIterInit, A: 3, B: 2, C: format12Next},
		"iter init by ref location iterator": {Op: OpIterInit, Aux: uint16(IterByRef), A: 4, B: 2, C: format12Next},
		"iter next by ref aux high":          {Op: OpIterNext, Aux: 4 | uint16(IterByRef), A: 1, B: 3, C: format12Next},
		"iter next by ref value at location": {Op: OpIterNext, Aux: uint16(IterByRef), A: 1, B: 2, C: format12Next},
		"iter key by ref":                    {Op: OpIterKey, Aux: uint16(IterByRef), A: 3, B: 1},
		"fetch unset for ref":                {Op: OpFetchDim, Aux: uint16(FetchUnset | FetchForRef), A: 4, B: 2, C: 0},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format12Image(in), ErrInvalid) })
	}
	t.Run("bind with a value empties the register", func(t *testing.T) {
		image := format12Image(Instruction{Op: OpBind, Aux: uint16(BindValue), A: 2, B: 0}, Instruction{Op: OpOutput, A: 0})
		_, err := Build(context.Background(), image, testOptions())
		var e *Error
		if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, "read before definition") {
			t.Fatalf("error = %v, want a read before definition", err)
		}
	})
}

func TestFormat13IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, in := range format13ValidInstructions() {
		for _, minor := range []uint16{0, 1, 2} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format12Image(in)
				if p, err := loadBytes(encodeAtMinor(t, image, 3)); err != nil || p == nil {
					t.Fatalf("minor 3 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || e.PC != format12Body {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid at the instruction", minor, p, err)
				}
				if !strings.Contains(e.Detail, "format 1.") {
					t.Fatalf("minor %d error = %+v, want the formats", minor, e)
				}
			})
		}
	}
	t.Run("a format 1.2 file still loads and its instructions keep their meaning", func(t *testing.T) {
		if p, err := loadBytes(encodeAtMinor(t, format12Everything(), 2)); err != nil || p == nil {
			t.Fatalf("Load=(%v,%v)", p, err)
		}
	})
}

func TestFormat13Safety(t *testing.T) {
	want := Safety{Steps: 1, CheckCancel: true, MayAllocate: true, MayThrow: true, MayCallUser: true}
	for _, op := range []Opcode{OpBind, OpMakeRef} {
		if s, ok := op.Safety(); !ok || s != want {
			t.Errorf("Safety(%d) = %+v, %v; want %+v", op, s, ok, want)
		}
	}
	if OpBind != 42 || OpMakeRef != 43 {
		t.Errorf("format 1.3 opcodes are renumbered: %d %d", OpBind, OpMakeRef)
	}
}

// An element location may be bound, made a reference or iterated by
// reference right after the FetchDim that defined it, and nowhere else.
func TestFormat13ElementLocations(t *testing.T) {
	fetch := Instruction{Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef), A: 4, B: 2, C: 0}
	for name, body := range map[string][]Instruction{
		"variable bound to the element": {fetch, {Op: OpBind, A: 2, B: 4}},
		"element bound to a variable":   {fetch, {Op: OpBind, A: 4, B: 2}},
		"element bound to a reference":  {{Op: OpMakeRef, A: 3, B: 2}, fetch, {Op: OpBind, Aux: uint16(BindValue), A: 4, B: 3}},
		"element made a reference":      {fetch, {Op: OpMakeRef, A: 3, B: 4}},
		"element iterated by reference": {fetch, {Op: OpIterInit, Aux: uint16(IterByRef), A: 3, B: 4, C: format12Body + 2}},
		"nested fetch then bind":        {fetch, {Op: OpConst, A: 0, B: 0}, {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef), A: 4, B: 4, C: 0}, {Op: OpBind, A: 2, B: 4}},
	} {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format12Image(body...)) })
	}
	for name, body := range map[string][]Instruction{
		"bind after the sequence ended":     {fetch, {Op: OpOutput, A: 0}, {Op: OpBind, A: 2, B: 4}},
		"element bound to itself":           {fetch, {Op: OpBind, A: 4, B: 4}},
		"make ref after the sequence":       {fetch, {Op: OpOutput, A: 0}, {Op: OpMakeRef, A: 3, B: 4}},
		"two binds through the element":     {fetch, {Op: OpBind, A: 2, B: 4}, {Op: OpBind, A: 4, B: 2}},
		"by-value iterator over a location": {fetch, {Op: OpIterInit, A: 3, B: 4, C: format12Body + 2}},
	} {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format12Image(body...), ErrInvalid) })
	}
}
