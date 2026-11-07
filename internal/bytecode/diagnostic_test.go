package bytecode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/internal/diagcodes"
)

func TestDiagnoseSentinelsAndErrors(t *testing.T) {
	for _, c := range []struct {
		err  error
		code diag.Code
	}{
		{ErrInvalid, diagcodes.BytecodeInvalid}, {ErrLimit, diagcodes.BytecodeLimit},
		{ErrVersion, diagcodes.BytecodeVersion}, {ErrProfile, diagcodes.BytecodeProfile},
		{ErrIntegrity, diagcodes.BytecodeIntegrity}, {ErrTrust, diagcodes.BytecodeTrust},
		{ErrCapability, diagcodes.BytecodeCapability},
		{invalidAt(12, "invalid header length"), diagcodes.BytecodeInvalid},
		{fail(ErrLimit, "too many functions"), diagcodes.BytecodeLimit},
	} {
		d, ok := Diagnose(c.err)
		if !ok || d.Code != c.code || d.Message == "" || len(d.Help) == 0 {
			t.Errorf("Diagnose(%v) = %+v, %v; want code %s", c.err, d, ok, c.code)
		}
	}
	if _, ok := Diagnose(errors.New("other")); ok {
		t.Fatal("foreign error diagnosed")
	}
}

// A verification error inside a function points at the instruction's source
// position, so the host can show the PHP source line.
func TestBuildErrorCarriesSourcePosition(t *testing.T) {
	image := validImage()
	image.Sources = []Source{{Name: "/app/a.php", Digest: sha256.Sum256([]byte("<?php\necho 1;\n")), Bytes: 14, Lines: 2}}
	f := &image.Functions[0]
	f.Code = append([]Instruction{{Op: 0xff}}, f.Code...)
	f.Positions = append([]Position{{Source: 1, Start: 6, End: 10, Line: 2}}, f.Positions...)
	_, err := Build(context.Background(), image, Options{Limits: DefaultLimits(), Supported: AllCapabilities})
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("got %v", err)
	}
	if e.Source != image.Sources[0].Name || e.Pos.Line != 2 || e.Pos.Start != 6 {
		t.Fatalf("error lacks the source position: %+v", e)
	}
	d, _ := Diagnose(err)
	var b bytes.Buffer
	src := map[string]string{image.Sources[0].Name: "<?php\necho 1;\n"}
	if err := diag.Render(&b, d, diag.Options{Source: func(n string) ([]byte, bool) { s, ok := src[n]; return []byte(s), ok }}); err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + b.String())
	if d.Primary.Line != 2 || d.Primary.Offset != 6 || d.Primary.Length != 4 || d.Primary.File != image.Sources[0].Name {
		t.Fatalf("primary span %+v", d.Primary)
	}
}

// locate must not attach a position that verification rejects.
func TestReviewLocateSkipsOutOfRangePositions(t *testing.T) {
	src := Source{Name: "/app/a.php", Digest: sha256.Sum256([]byte("x")), Bytes: 14, Lines: 2}
	image := Image{Sources: []Source{src}, Functions: []Function{{Positions: []Position{
		{Source: 1, Start: 0, End: 4, Line: 9}, {Source: 1, Start: 0, End: 999, Line: 1}, {Source: 1, Start: 2, End: 4, Line: 1},
	}}}}
	for pc, want := range []bool{false, false, true} {
		err := locate(image, &Error{Kind: ErrInvalid, Function: 0, PC: pc, Offset: -1})
		var e *Error
		errors.As(err, &e)
		if got := e.Pos.Line != 0; got != want {
			t.Errorf("pc %d: located %v, want %v (%+v)", pc, got, want, e.Pos)
		}
	}
}

func TestReviewDiagnoseJoinedIsDeterministic(t *testing.T) {
	err := errors.Join(ErrLimit, ErrTrust)
	first, _ := Diagnose(err)
	for i := 0; i < 200; i++ {
		if d, _ := Diagnose(err); d.Code != first.Code {
			t.Fatalf("run %d gave %s after %s", i, d.Code, first.Code)
		}
	}
}
