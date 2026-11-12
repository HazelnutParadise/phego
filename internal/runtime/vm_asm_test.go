package runtime

import (
	"context"
	"crypto/sha256"
	"math"
	"testing"
	"time"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// vmAsm builds verified stage-1 images by hand, following the compiler
// conventions: one Location register per local defined in the prologue,
// parameters stored from the first registers, every instruction positioned.
type vmAsm struct {
	t      testing.TB
	width  int
	file   string
	consts []bytecode.Constant
	strs   map[string]uint32
	funcs  []*vmFn
	// required is the image's capability set, for unsupported-opcode tests.
	required bytecode.CapabilitySet
}

type vmFn struct {
	a      *vmAsm
	name   string
	params int
	regs   []bytecode.RegisterKind
	code   []bytecode.Instruction
	pos    []bytecode.Position
	names  []uint32
	locs   map[string]uint32 // local name -> location register
	index  map[string]uint32 // local name -> local index
	line   uint32
}

const vmTestFile = "/w/t.php"

func newAsm(t testing.TB, width int) *vmAsm {
	a := &vmAsm{t: t, width: width, file: vmTestFile, strs: map[string]uint32{}}
	return a
}

func (a *vmAsm) constant(c bytecode.Constant) uint32 {
	if c.Kind == bytecode.String {
		if i, ok := a.strs[c.Bytes]; ok {
			return i
		}
		a.strs[c.Bytes] = uint32(len(a.consts))
	}
	a.consts = append(a.consts, c)
	return uint32(len(a.consts) - 1)
}

func (a *vmAsm) str(s string) uint32 {
	return a.constant(bytecode.Constant{Kind: bytecode.String, Bytes: s})
}

// fn declares a function; the first one is {main}. locals lists the PHP
// variables (parameters first); hidden temporaries use "".
func (a *vmAsm) fn(name string, params int, line uint32, locals ...string) *vmFn {
	f := &vmFn{a: a, name: name, params: params, locs: map[string]uint32{}, index: map[string]uint32{}, line: line}
	for i := 0; i < params; i++ {
		f.regs = append(f.regs, bytecode.Value)
	}
	for i, l := range locals {
		r := f.reg(bytecode.Location)
		f.names = append(f.names, a.str(l))
		if l != "" {
			f.locs[l] = r
			f.index[l] = uint32(i)
		}
		f.emit(bytecode.OpLocal, 0, r, uint32(i), 0)
	}
	for i := 0; i < params; i++ {
		f.emit(bytecode.OpStore, 0, f.locs[locals[i]], uint32(i), 0)
	}
	a.funcs = append(a.funcs, f)
	return f
}

func (f *vmFn) reg(k bytecode.RegisterKind) uint32 {
	f.regs = append(f.regs, k)
	return uint32(len(f.regs) - 1)
}

func (f *vmFn) at(line uint32) *vmFn { f.line = line; return f }

func (f *vmFn) emit(op bytecode.Opcode, aux uint16, a, b, c uint32) int {
	f.code = append(f.code, bytecode.Instruction{Op: op, Aux: aux, A: a, B: b, C: c})
	f.pos = append(f.pos, bytecode.Position{Source: 1, Line: f.line})
	return len(f.code) - 1
}

func (f *vmFn) pc() uint32 { return uint32(len(f.code)) }

// k loads a constant into a fresh Value register: nil, bool, int, int64,
// float64 or string.
func (f *vmFn) k(v any) uint32 {
	var c bytecode.Constant
	switch x := v.(type) {
	case nil:
		c = bytecode.Constant{Kind: bytecode.Null}
	case bool:
		c = bytecode.Constant{Kind: bytecode.Bool}
		if x {
			c.Bits = 1
		}
	case int:
		c = bytecode.Constant{Kind: bytecode.Int, Bits: uint64(int64(x))}
	case int64:
		c = bytecode.Constant{Kind: bytecode.Int, Bits: uint64(x)}
	case float64:
		c = bytecode.Constant{Kind: bytecode.Float, Bits: math.Float64bits(x)}
	case string:
		c = bytecode.Constant{Kind: bytecode.String, Bytes: x}
	default:
		f.a.t.Fatalf("unsupported constant %T", v)
	}
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpConst, 0, r, f.a.constant(c), 0)
	return r
}

func (f *vmFn) loc(name string) uint32 {
	r, ok := f.locs[name]
	if !ok {
		f.a.t.Fatalf("unknown local %q", name)
	}
	return r
}

func (f *vmFn) load(name string) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpLoad, 0, r, f.loc(name), 0)
	return r
}

func (f *vmFn) loadQuiet(name string) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpLoad, 1, r, f.loc(name), 0)
	return r
}

func (f *vmFn) store(name string, v uint32) { f.emit(bytecode.OpStore, 0, f.loc(name), v, 0) }

func (f *vmFn) bin(op bytecode.BinaryOp, x, y uint32) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpBinary, uint16(op), r, x, y)
	return r
}

func (f *vmFn) unary(op bytecode.UnaryOp, x uint32) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpUnary, uint16(op), r, x, 0)
	return r
}

func (f *vmFn) cast(k bytecode.CastKind, x uint32) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpCast, uint16(k), r, x, 0)
	return r
}

func (f *vmFn) incdec(op bytecode.IncDecOp, name string) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpIncDec, uint16(op), r, f.loc(name), 0)
	return r
}

func (f *vmFn) echo(v uint32) { f.emit(bytecode.OpOutput, 0, v, 0, 0) }

func (f *vmFn) echoK(s string) { f.echo(f.k(s)) }

func (f *vmFn) ret(v uint32) { f.emit(bytecode.OpReturn, 0, v, 0, 0) }

func (f *vmFn) retNull() { f.ret(f.k(nil)) }

func (f *vmFn) exit() { f.emit(bytecode.OpExit, 0, 0, 0, 0) }

func (f *vmFn) exitWith(v uint32) { f.emit(bytecode.OpExit, 1, v, 0, 0) }

func (f *vmFn) newArray() uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpNewArray, 0, r, 0, 0)
	return r
}

func (f *vmFn) get(arr, key uint32, quiet bool) uint32 {
	r := f.reg(bytecode.Value)
	aux := uint16(0)
	if quiet {
		aux = 1
	}
	f.emit(bytecode.OpArrayGet, aux, r, arr, key)
	return r
}

func (f *vmFn) set(name string, key, v uint32) { f.emit(bytecode.OpArraySet, 0, f.loc(name), key, v) }

func (f *vmFn) push(name string, v uint32) { f.emit(bytecode.OpArrayAppend, 0, f.loc(name), v, 0) }

func (f *vmFn) unsetKey(name string, key uint32) {
	f.emit(bytecode.OpArrayUnset, 0, f.loc(name), key, 0)
}

func (f *vmFn) isset(name string) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpIsset, 0, r, f.loc(name), 0)
	return r
}

func (f *vmFn) issetKey(arr, key uint32) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpArrayIsset, 0, r, arr, key)
	return r
}

func (f *vmFn) unset(name string) { f.emit(bytecode.OpUnset, 0, f.loc(name), 0, 0) }

// call calls function index fi with positional arguments.
func (f *vmFn) call(fi int, args ...uint32) uint32 {
	for _, a := range args {
		f.emit(bytecode.OpPush, 0, a, 0, 0)
	}
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpCall, 0, r, uint32(fi), uint32(len(args)))
	return r
}

func (f *vmFn) callName(name string, args ...uint32) uint32 {
	n := f.k(name)
	for _, a := range args {
		f.emit(bytecode.OpPush, 0, a, 0, 0)
	}
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpCallValue, 0, r, n, uint32(len(args)))
	return r
}

// jump emits a Jump to patch later; branch a Branch with both targets open.
func (f *vmFn) jump(target uint32) int { return f.emit(bytecode.OpJump, 0, target, 0, 0) }

func (f *vmFn) branch(cond uint32) int { return f.emit(bytecode.OpBranch, 0, cond, 0, 0) }

func (f *vmFn) patch(pc int, b, c uint32) {
	in := &f.code[pc]
	switch in.Op {
	case bytecode.OpJump:
		in.A = b
	case bytecode.OpBranch:
		in.B, in.C = b, c
	}
}

// arrayOf builds a list literal as the compiler does: NewArray, then one
// ArrayAdd per value.
func (f *vmFn) arrayOf(values ...uint32) uint32 {
	r := f.newArray()
	for _, v := range values {
		f.emit(bytecode.OpArrayAdd, 0, r, v, 0)
	}
	return r
}

// add adds key => v to the literal in register arr (ArrayAdd with AddKey).
func (f *vmFn) add(arr, key, v uint32) {
	f.emit(bytecode.OpArrayAdd, uint16(bytecode.AddKey), arr, v, key)
}

// addVarKey adds $name => v, the key read by the instruction itself.
func (f *vmFn) addVarKey(arr uint32, name string, v uint32) {
	f.emit(bytecode.OpArrayAdd, uint16(bytecode.AddKey|bytecode.AddKeyLocation), arr, v, f.loc(name))
}

// setVars is $name[key] = value where the key and/or the value are variables
// read by ArraySet itself; an empty keyVar or valueVar uses the register.
func (f *vmFn) setVars(name, keyVar string, key uint32, valueVar string, v uint32) {
	var flags bytecode.ArrayWriteFlags
	if keyVar != "" {
		flags |= bytecode.KeyLocation
		key = f.loc(keyVar)
	}
	if valueVar != "" {
		flags |= bytecode.ValueLocation
		v = f.loc(valueVar)
	}
	f.emit(bytecode.OpArraySet, uint16(flags), f.loc(name), key, v)
}

// pushVar is $name[] = $valueVar, the value read by ArrayAppend itself.
func (f *vmFn) pushVar(name, valueVar string) {
	f.emit(bytecode.OpArrayAppend, uint16(bytecode.ValueLocation), f.loc(name), f.loc(valueVar), 0)
}

// unsetVar is unset($name[$keyVar]), the key read by ArrayUnset itself.
func (f *vmFn) unsetVar(name, keyVar string) {
	f.emit(bytecode.OpArrayUnset, uint16(bytecode.KeyLocation), f.loc(name), f.loc(keyVar), 0)
}

// hidden adds a compiler temporary local; it needs its Local at the call
// site, which it emits.
func (f *vmFn) hidden() uint32 {
	idx := uint32(len(f.names))
	f.names = append(f.names, f.a.str(""))
	r := f.reg(bytecode.Location)
	f.emit(bytecode.OpLocal, 0, r, idx, 0)
	return r
}

func vmIdentity(width int) bytecode.Identity {
	d := func(s string) [32]byte { return sha256.Sum256([]byte(s)) }
	return bytecode.Identity{Profile: profile.Profile{Release: "8.5.12", IntWidth: width}, GuestPlatform: "php-8.5.12", Target: "portable-go",
		Source: d("source"), Compiler: d("phego/internal/vm stage-1 format-1.1"), Modules: d("phego/modules/none"),
		RuntimeABI: d("phego/runtime-abi/w17-1"), Policy: d("phego/policy/unbound"), Scope: d("phego/scope/engine")}
}

func (a *vmAsm) image() bytecode.Image {
	img := bytecode.Image{Identity: vmIdentity(a.width), Required: a.required, Constants: append([]bytecode.Constant(nil), a.consts...),
		Sources: []bytecode.Source{{Name: a.file, Digest: sha256.Sum256([]byte(a.file)), Bytes: 0, Lines: 1 << 20}}}
	maxStack := uint32(0)
	for _, f := range a.funcs {
		stack, depth := uint32(0), uint32(0)
		for _, in := range f.code {
			switch in.Op {
			case bytecode.OpPush:
				depth++
				stack = max(stack, depth)
			case bytecode.OpCall, bytecode.OpCallValue:
				depth -= in.C
			}
		}
		maxStack = max(maxStack, stack)
		img.Functions = append(img.Functions, bytecode.Function{Name: f.name, Params: uint32(f.params), Locals: uint32(len(f.names)),
			MaxStack: stack, ReturnKind: bytecode.Value, Registers: f.regs, Code: f.code, Positions: f.pos, LocalNames: f.names})
	}
	_ = maxStack
	return img
}

func (a *vmAsm) program() *Program {
	a.t.Helper()
	p, err := bytecode.Build(context.Background(), a.image(), bytecode.Options{Limits: bytecode.DefaultLimits(), Supported: bytecode.AllCapabilities})
	if err != nil {
		a.t.Fatalf("Build: %v", err)
	}
	prog, err := NewProgram(p)
	if err != nil {
		a.t.Fatalf("NewProgram: %v", err)
	}
	return prog
}

func readyRequest(t testing.TB) *Request {
	t.Helper()
	r, err := NewRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	return r
}

var vmCleanup, _ = context.WithTimeout(context.Background(), 24*time.Hour)

// vmConfig is the default test configuration: display and log on,
// error_reporting E_ALL, generous limits.
func vmConfig(t testing.TB, width int) RunConfig {
	return RunConfig{Context: context.Background(), Cleanup: vmCleanup, Request: readyRequest(t), IntWidth: width,
		Depth: 256, Callbacks: 1 << 20, OutputBytes: 1 << 24, DisplayErrors: true, LogErrors: true, ErrorReporting: -1}
}

func runAsm(t testing.TB, a *vmAsm) RunResult {
	t.Helper()
	return a.program().Run(vmConfig(t, a.width))
}
