package bytecode

import (
	"errors"
	"fmt"

	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/internal/diagcodes"
)

var kindCodes = map[error]diag.Code{
	ErrInvalid: diagcodes.BytecodeInvalid, ErrLimit: diagcodes.BytecodeLimit,
	ErrVersion: diagcodes.BytecodeVersion, ErrProfile: diagcodes.BytecodeProfile,
	ErrIntegrity: diagcodes.BytecodeIntegrity, ErrTrust: diagcodes.BytecodeTrust,
	ErrCapability: diagcodes.BytecodeCapability,
}

// Diagnose describes a Build or Load failure, including the bare sentinel
// errors returned before any position is known. It reports false for errors
// that did not come from this package, such as context cancellation.
func Diagnose(err error) (diag.Diagnostic, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Diagnostic(), true
	}
	for kind := range kindCodes {
		if errors.Is(err, kind) {
			return (&Error{Kind: kind, Offset: -1, Function: -1, PC: -1}).Diagnostic(), true
		}
	}
	return diag.Diagnostic{}, false
}

// Diagnostic describes e for diag.Render. With a source position the primary
// span covers the instruction's source bytes; otherwise it names the byte
// offset or the function and instruction that failed verification.
func (e *Error) Diagnostic() diag.Diagnostic {
	code, ok := kindCodes[e.Kind]
	if !ok {
		code = diagcodes.BytecodeInvalid
	}
	entry := diagcodes.Registry[code]
	msg := e.Detail
	if msg == "" {
		msg = fmt.Sprint(e.Kind)
	}
	d := diag.Diagnostic{Code: code, Message: msg, Notes: []string{entry.Note}, Help: []string{entry.Help}}
	where := e.where()
	if e.Pos.Line > 0 && e.Source != "" {
		d.Primary = diag.Span{File: e.Source, Line: int(e.Pos.Line), Offset: int(e.Pos.Start), Length: int(e.Pos.End - e.Pos.Start)}
		if where != "" {
			d.Notes = append(d.Notes, "the failing instruction is "+where)
		}
		return d
	}
	if where == "" {
		where = "bytecode"
	} else {
		where = "bytecode " + where
	}
	d.Primary = diag.Span{File: where}
	return d
}

func (e *Error) where() string {
	switch {
	case e.Function >= 0 && e.PC >= 0:
		return fmt.Sprintf("function %d, instruction %d", e.Function, e.PC)
	case e.Function >= 0:
		return fmt.Sprintf("function %d", e.Function)
	case e.Offset >= 0:
		return fmt.Sprintf("byte %d", e.Offset)
	}
	return ""
}

// locate adds the source position of the failing instruction to a Build
// error. Positions are checked by verification, so an out-of-range or
// synthetic position is simply left out.
func locate(image Image, err error) error {
	var e *Error
	if !errors.As(err, &e) || e.Function < 0 || e.PC < 0 || e.Function >= len(image.Functions) {
		return err
	}
	f := image.Functions[e.Function]
	if e.PC >= len(f.Positions) {
		return err
	}
	pos := f.Positions[e.PC]
	if pos.Source == 0 || uint64(pos.Source) > uint64(len(image.Sources)) || pos.Line == 0 || pos.End < pos.Start {
		return err
	}
	cp := *e
	cp.Source, cp.Pos = image.Sources[pos.Source-1].Name, pos
	return &cp
}
