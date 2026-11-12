package bytecode

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// localNamesImage is validCallAndReferenceImage with a constant pool holding
// variable names and names on both functions. Function 1 (callee) is last in
// the payload, so its local names table ends the encoding.
func localNamesImage() Image {
	i := validCallAndReferenceImage()
	i.Constants = []Constant{{Kind: Null}, {Kind: String, Bytes: "x"}, {Kind: String, Bytes: "y"}, {Kind: Int, Bits: 7}, {Kind: String}}
	i.Functions[0].LocalNames = []uint32{1}
	i.Functions[1].LocalNames = []uint32{2}
	return i
}

func TestLocalNamesBuildAcceptsValidTables(t *testing.T) {
	cases := map[string]func(*Image){
		"no names":        func(i *Image) { i.Functions[0].LocalNames, i.Functions[1].LocalNames = nil, nil },
		"names":           func(*Image) {},
		"empty table":     func(i *Image) { i.Functions[1].LocalNames = []uint32{} },
		"duplicate names": func(i *Image) { i.Functions[1].LocalNames = []uint32{1} },
		"empty name":      func(i *Image) { i.Functions[1].LocalNames = []uint32{4} },
		"several locals":  func(i *Image) { i.Functions[1].Locals = 3; i.Functions[1].LocalNames = []uint32{1, 2, 1} },
		"names on one fn": func(i *Image) { i.Functions[0].LocalNames = nil },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			i := localNamesImage()
			edit(&i)
			requireValidBuild(t, i)
		})
	}
}

func TestLocalNamesBuildRejectsInvalidTables(t *testing.T) {
	cases := map[string]struct {
		names  []uint32
		detail string
	}{
		"too many":           {[]uint32{1, 2}, "local name count"},
		"too few":            {nil, ""}, // set below with Locals 2
		"index out of range": {[]uint32{5}, "local name index"},
		"far out of range":   {[]uint32{0xffffffff}, "local name index"},
		"null constant":      {[]uint32{0}, "not a string"},
		"int constant":       {[]uint32{3}, "not a string"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			i := localNamesImage()
			i.Functions[1].LocalNames = tc.names
			detail := tc.detail
			if name == "too few" {
				i.Functions[1].Locals = 2
				i.Functions[1].LocalNames = []uint32{1}
				detail = "local name count"
			}
			requireBuildError(t, i, ErrInvalid)
			_, err := Build(context.Background(), i, testOptions())
			var e *Error
			if !errors.As(err, &e) || e.Function != 1 || !strings.Contains(e.Detail, detail) {
				t.Fatalf("error = %+v, want function 1 and detail containing %q", e, detail)
			}
		})
	}
}

func TestLocalNamesEncodeLoadRoundTrip(t *testing.T) {
	image := localNamesImage()
	data := encodeAtMinor(t, image, FormatMinor)
	p, err := loadBytes(data)
	if err != nil || p == nil {
		t.Fatalf("Load=(%v,%v)", p, err)
	}
	for fi, want := range []string{"x", "y"} {
		view, _ := p.Function(uint32(fi))
		if name, ok := view.LocalName(0); !ok || name != want {
			t.Fatalf("function %d LocalName(0) = %q, %v; want %q", fi, name, ok, want)
		}
	}
	again, err := p.Encode(context.Background())
	if err != nil || string(again) != string(data) {
		t.Fatalf("canonical re-encode differs: err=%v", err)
	}
	// The callee's one-entry table ends the payload: count 1, then index 2.
	if binary.LittleEndian.Uint32(data[len(data)-8:]) != 1 || binary.LittleEndian.Uint32(data[len(data)-4:]) != 2 {
		t.Fatalf("payload does not end with the callee local names table: % x", data[len(data)-8:])
	}
	noNames := localNamesImage()
	noNames.Functions[0].LocalNames, noNames.Functions[1].LocalNames = nil, nil
	if plain := encodeAtMinor(t, noNames, FormatMinor); binary.LittleEndian.Uint32(plain[len(plain)-4:]) != 0 {
		t.Fatal("Encode did not write an empty local names table")
	}
}

func TestLocalNamesWireRejectsInvalidTables(t *testing.T) {
	original := encodeAtMinor(t, localNamesImage(), FormatMinor)
	cases := map[string]struct {
		edit   func([]byte) []byte
		detail string
	}{
		"index out of range": {func(b []byte) []byte { binary.LittleEndian.PutUint32(b[len(b)-4:], 99); return b }, "local name index"},
		"non-string":         {func(b []byte) []byte { binary.LittleEndian.PutUint32(b[len(b)-4:], 3); return b }, "not a string"},
		"count not locals": {func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[len(b)-8:], 2)
			return append(b, 1, 0, 0, 0)
		}, "local name count"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			b := tc.edit(append([]byte(nil), original...))
			repairWireIntegrity(b)
			p, err := loadBytes(b)
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || e.Function != 1 || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Load=(%v,%+v), want ErrInvalid in function 1 with %q", p, e, tc.detail)
			}
		})
	}
	t.Run("truncated table", func(t *testing.T) {
		b := append([]byte(nil), original[:len(original)-4]...)
		repairWireIntegrity(b)
		if p, err := loadBytes(b); p != nil || !errors.Is(err, ErrInvalid) {
			t.Fatalf("Load=(%v,%v), want ErrInvalid", p, err)
		}
	})
}

func TestLocalNamesMinorZeroHasNoTable(t *testing.T) {
	image := localNamesImage()
	one := encodeAtMinor(t, image, 1)
	zero := encodeAtMinor(t, image, 0)
	// Two functions, each losing a u32 count and one u32 index.
	if len(one)-len(zero) != 2*(4+4) {
		t.Fatalf("minor 0 is %d bytes shorter, want 16", len(one)-len(zero))
	}
	p, err := loadBytes(zero)
	if err != nil || p == nil {
		t.Fatalf("minor 0 Load=(%v,%v)", p, err)
	}
	for fi := uint32(0); fi < 2; fi++ {
		view, _ := p.Function(fi)
		if name, ok := view.LocalName(0); ok || name != "" {
			t.Fatalf("minor 0 function %d LocalName = %q, %v", fi, name, ok)
		}
	}
	// A minor-0 header in front of a minor-1 payload misparses the tables
	// as the next record, which is rejected (invalid or over a limit).
	b := append([]byte(nil), one...)
	binary.LittleEndian.PutUint16(b[10:12], 0)
	repairWireIntegrity(b)
	if p, err := loadBytes(b); p != nil || err == nil {
		t.Fatalf("minor 0 header with tables Load=(%v,%v), want an error", p, err)
	}
}

func TestLocalNameAccessorAndOwnership(t *testing.T) {
	image := localNamesImage()
	image.Functions[1].Locals = 3
	image.Functions[1].LocalNames = []uint32{1, 2, 4}
	p := requireValidBuild(t, image)
	view, _ := p.Function(1)
	for idx, want := range []string{"x", "y", ""} {
		if name, ok := view.LocalName(uint32(idx)); !ok || name != want {
			t.Fatalf("LocalName(%d) = %q, %v; want %q", idx, name, ok, want)
		}
	}
	for _, idx := range []uint32{3, 0xffffffff} {
		if name, ok := view.LocalName(idx); ok || name != "" {
			t.Fatalf("LocalName(%d) = %q, %v; want out of range", idx, name, ok)
		}
	}
	if name, ok := (FunctionView{}).LocalName(0); ok || name != "" {
		t.Fatalf("zero FunctionView LocalName = %q, %v", name, ok)
	}
	noNames := localNamesImage()
	noNames.Functions[1].LocalNames = nil
	if v, _ := requireValidBuild(t, noNames).Function(1); func() bool { _, ok := v.LocalName(0); return ok }() {
		t.Fatal("function without names reported a name")
	}
	// Changing the caller's image after Build does not reach the Program.
	image.Functions[1].LocalNames[0] = 2
	image.Constants[1].Bytes = "changed"
	image.Constants[2] = Constant{Kind: Int}
	if name, _ := view.LocalName(0); name != "x" {
		t.Fatalf("LocalName after mutating input = %q, want x", name)
	}
	if name, _ := view.LocalName(1); name != "y" {
		t.Fatalf("LocalName after mutating input = %q, want y", name)
	}
	// Neither do changes to the copy returned by Constants.
	consts := p.Constants(nil)
	consts[1].Bytes = "other"
	if name, _ := view.LocalName(0); name != "x" {
		t.Fatalf("LocalName after mutating Constants copy = %q", name)
	}
}
