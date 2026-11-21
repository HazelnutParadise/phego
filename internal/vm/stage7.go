package vm

import (
	"sort"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 7A: classes and objects. REQ-001, REQ-016, REQ-025, REQ-026;
// T-009, T-010, T-029..T-032. The compiler emits the class table and the
// format 1.7 instructions in the order zend_compile_class_decl,
// zend_compile_new, zend_compile_prop, zend_compile_method_call and
// zend_compile_static_call emit their opcodes; what an instruction does to
// an object is the runtime's (W-13).

// --- the class table ---

func memberFlags(mods []string) bytecode.MemberFlags {
	var f bytecode.MemberFlags
	for _, m := range mods {
		switch m {
		case "protected":
			f |= bytecode.MemberProtected
		case "private":
			f |= bytecode.MemberPrivate
		case "static":
			f |= bytecode.MemberStatic
		case "final":
			f |= bytecode.MemberFinal
		case "abstract":
			f |= bytecode.MemberAbstract
		case "readonly":
			f |= bytecode.MemberReadonly
		}
	}
	return f
}

func hasMod(mods []string, want string) bool {
	for _, m := range mods {
		if m == want {
			return true
		}
	}
	return false
}

// classUnsupported reports the first part of a class declaration this stage
// cannot compile.
func (c *compiler) classUnsupported(cl *ir.Class) *UnsupportedError {
	fail := func(feature string, line int) *UnsupportedError {
		return &UnsupportedError{Feature: feature, File: c.unit.File, Line: positive(line, positive(cl.Line, 1))}
	}
	if isBuiltinClass(cl.Name) && !cl.Hoisted {
		// PHP raises this redeclaration when the statement runs.
		return fail("declaring a class with the name of builtin class "+cl.Name, cl.Line)
	}
	for _, p := range cl.Props {
		if p.Default != nil && createsClosure([]*ir.Node{p.Default}) {
			// The default would be an object, which a property's
			// default cannot hold yet.
			return fail("closure in a property default", positive(p.Default.Line, cl.Line))
		}
	}
	names := append([]*ir.Name{}, cl.Interfaces...)
	if cl.Parent != nil {
		names = append([]*ir.Name{cl.Parent}, names...)
	}
	for _, n := range append(names, cl.Traits...) {
		lower := phpname.Lower(n.Resolved)
		if !c.declaredClass[lower] && isBuiltinClass(lower) && lower != "stringable" && lower != "closure" {
			return fail("extending or implementing builtin class "+n.Resolved, cl.Line)
		}
	}
	switch {
	case cl.Kind == ir.KindEnum:
		return fail("enum declaration", cl.Line)
	case cl.Anonymous:
		return fail("anonymous class", cl.Line)
	}
	for _, a := range cl.Attributes {
		if !allowDynamicAttribute(a) {
			return fail("class attribute", a.Line)
		}
	}
	for _, k := range cl.Consts {
		if len(k.Attributes) > 0 {
			return fail("class constant attribute", k.Attributes[0].Line)
		}
	}
	for _, p := range cl.Props {
		switch {
		case p.HasHooks || len(p.Hooks) > 0:
			return fail("property hook", p.Line)
		case len(p.Attributes) > 0 && !p.Promoted:
			return fail("property attribute", p.Attributes[0].Line)
		}
		for _, m := range p.Modifiers {
			if len(m) > 5 && m[len(m)-5:] == "(set)" {
				return fail("asymmetric property visibility", p.Line)
			}
		}
	}
	for _, m := range cl.Methods {
		if len(m.Attributes) > 0 {
			return fail("method attribute", m.Attributes[0].Line)
		}
		if m.ByRef {
			return fail("return by reference", m.Line)
		}
	}
	return nil
}

// classCompileError reports a declaration PHP rejects while compiling the
// class (zend_begin_method_decl, zend_compile_prop_group) that the
// lowering lets through.
func (c *compiler) classCompileError(cl *ir.Class) *CompileError {
	fail := func(line int, msg string) *CompileError {
		return &CompileError{Message: msg, File: c.unit.File, Line: positive(line, positive(cl.Line, 1))}
	}
	if cl.Hoisted && isBuiltinClass(cl.Name) {
		// zend_compile_class_decl binds the class while compiling and finds
		// the name taken by a class of the engine.
		i := sort.SearchStrings(builtinClasses, phpname.Lower(cl.Name))
		return fail(cl.Line, "Cannot redeclare "+builtinClassDecl[i][1]+" "+builtinClassDecl[i][0])
	}
	if cl.Kind == ir.KindInterface {
		for _, p := range cl.Props {
			if !p.HasHooks {
				return fail(p.Line, "Interfaces may only include hooked properties")
			}
		}
	}
	for _, m := range cl.Methods {
		switch {
		case cl.Kind == ir.KindInterface && (hasMod(m.Modifiers, "private") || hasMod(m.Modifiers, "protected")):
			return fail(m.Line, "Access type for interface method "+cl.Name+"::"+m.Name+"() must be public")
		case hasMod(m.Modifiers, "abstract") && m.HasBody:
			return fail(m.Line, "Abstract function "+cl.Name+"::"+m.Name+"() cannot contain body")
		}
	}
	return nil
}

// allowDynamicAttribute reports whether a is #[AllowDynamicProperties], the
// one class attribute this stage understands.
func allowDynamicAttribute(a ir.Attribute) bool {
	return a.Name != nil && phpname.EqualFold(a.Name.Resolved, "AllowDynamicProperties") && len(a.Arguments) == 0
}

// constValue compiles the value of a class constant or a property default:
// a scalar literal goes to the pool, any other constant expression becomes
// a function without parameters that the run time calls in the class's
// scope when the value is first needed.
func (c *compiler) constValue(cl *ir.Class, n *ir.Node, out *[]bytecode.Function, pending *[]*ir.Func) (value, init uint32) {
	if n.Op == ir.OpLiteral {
		return c.literal(n.Value) + 1, 0
	}
	f := &ir.Func{Kind: ir.FuncMethod, Name: "{constant expression}", Line: n.Line, Static: true, HasBody: true, Class: cl, Strict: c.unit.Strict,
		Body: []*ir.Node{{Op: ir.OpReturn, Line: n.Line, Kids: []*ir.Node{n}}}}
	*pending = append(*pending, f)
	return 0, uint32(len(*out)+len(*pending)-1) + 1
}

// classes builds the class table. funcs is the image's function list so
// far; the methods and initializers are appended to pending, in order, and
// compiled by the caller, which gives them the indices assigned here.
func (c *compiler) classTable(nfuncs int) (table []bytecode.Class, pending []*ir.Func, err *UnsupportedError) {
	out := make([]bytecode.Function, nfuncs) // only its length is used
	c.classIndex = make(map[*ir.Class]uint32, len(c.unit.Classes))
	c.declaredClass = make(map[string]bool, len(c.unit.Classes))
	for _, cl := range c.unit.Classes {
		c.declaredClass[phpname.Lower(cl.Name)] = true
	}
	for _, cl := range c.unit.Classes {
		if ce := c.classCompileError(cl); ce != nil {
			panic(ce)
		}
		if ue := c.classUnsupported(cl); ue != nil {
			if err == nil || ue.Line < err.Line {
				err = ue
			}
			continue
		}
		c.classIndex[cl] = uint32(len(table))
		bc := bytecode.Class{Name: c.stringConst(cl.Name), Line: uint32(positive(cl.Line, 1))}
		switch cl.Kind {
		case ir.KindInterface:
			bc.Flags |= bytecode.ClassInterface
		case ir.KindTrait:
			bc.Flags |= bytecode.ClassTrait
		}
		if hasMod(cl.Modifiers, "abstract") {
			bc.Flags |= bytecode.ClassAbstract
		}
		if hasMod(cl.Modifiers, "final") {
			bc.Flags |= bytecode.ClassFinal
		}
		if hasMod(cl.Modifiers, "readonly") {
			bc.Flags |= bytecode.ClassReadonly
		}
		if cl.Hoisted {
			bc.Flags |= bytecode.ClassHoisted
		}
		if len(cl.Attributes) > 0 {
			bc.Flags |= bytecode.ClassAllowDynamic
		}
		if cl.Parent != nil {
			bc.Parent = c.stringConst(cl.Parent.Resolved) + 1
		}
		for _, n := range cl.Interfaces {
			bc.Interfaces = append(bc.Interfaces, c.stringConst(n.Resolved))
		}
		for _, n := range cl.Traits {
			bc.Traits = append(bc.Traits, c.stringConst(n.Resolved))
		}
		for _, r := range cl.TraitRules {
			br := bytecode.TraitRule{Method: c.stringConst(r.Method), Line: uint32(positive(r.Line, 1))}
			if r.Trait != nil {
				br.Trait = c.stringConst(r.Trait.Resolved) + 1
			}
			for _, n := range r.Insteadof {
				br.Insteadof = append(br.Insteadof, c.stringConst(n.Resolved))
			}
			if len(r.Insteadof) == 0 {
				if r.Alias != "" {
					br.Alias = c.stringConst(r.Alias) + 1
				}
				if r.Modifier != "" {
					br.HasVisibility = true
					br.Flags = memberFlags([]string{r.Modifier}).Visibility()
				}
			}
			bc.Rules = append(bc.Rules, br)
		}
		for _, k := range cl.Consts {
			bk := bytecode.ClassConstant{Name: c.stringConst(k.Name), Flags: memberFlags(k.Modifiers) & (bytecode.MemberProtected | bytecode.MemberPrivate | bytecode.MemberFinal)}
			bk.Value, bk.Init = c.constValue(cl, k.Value, &out, &pending)
			bc.Constants = append(bc.Constants, bk)
		}
		for _, p := range cl.Props {
			bp := bytecode.Property{Name: c.stringConst(p.Name), Flags: memberFlags(p.Modifiers) & (bytecode.MemberProtected | bytecode.MemberPrivate | bytecode.MemberStatic | bytecode.MemberReadonly)}
			if p.Type != nil {
				bp.Type = c.stringConst(p.Type.Zend()) + 1
			}
			if p.Default != nil && !p.Promoted {
				bp.Value, bp.Init = c.constValue(cl, p.Default, &out, &pending)
			}
			bc.Properties = append(bc.Properties, bp)
		}
		for _, m := range cl.Methods {
			flags := memberFlags(m.Modifiers) & (bytecode.MemberProtected | bytecode.MemberPrivate | bytecode.MemberStatic | bytecode.MemberFinal | bytecode.MemberAbstract)
			if cl.Kind == ir.KindInterface || !m.HasBody {
				flags |= bytecode.MemberAbstract
			}
			if m.Static {
				flags |= bytecode.MemberStatic
			}
			pending = append(pending, m)
			bc.Methods = append(bc.Methods, bytecode.Method{
				Name:      c.stringConst(m.Name),
				Function:  uint32(len(out) + len(pending) - 1),
				Flags:     flags,
				Signature: c.stringConst(ir.MethodSignature(cl.Name, m)) + 1,
				Line:      uint32(positive(m.Line, 1)),
			})
		}
		table = append(table, bc)
	}
	return table, pending, err
}

// --- class and member operands ---

// classRef is the ClassRef and operand of a class reference: name, or the
// value of expr.
func (fc *fnCompiler) classRef(name *ir.Name, expr *ir.Node) (bytecode.ClassRef, uint32) {
	if name == nil {
		if expr == nil {
			panic(internalError{"class reference without a name or an expression"})
		}
		return bytecode.ClassValue, fc.value(expr)
	}
	if name.Special != "" && fc.f.Kind == ir.FuncNamed && fc.f.Class == nil {
		// zend_ensure_valid_class_fetch_type: a function's scope is known
		// while it is compiled, and it has none.
		panic(&CompileError{Message: `Cannot use "` + name.Special + `" when no class scope is active`, File: fc.c.unit.File, Line: positive(fc.cur, 1)})
	}
	switch name.Special {
	case "self":
		return bytecode.ClassSelf, 0
	case "parent":
		return bytecode.ClassParent, 0
	case "static":
		return bytecode.ClassStatic, 0
	}
	fc.c.checkBuiltinClass(name.Resolved, fc.cur, fc.instanceofOperand)
	return bytecode.ClassNamed, fc.c.stringConst(name.Resolved)
}

// checkBuiltinClass reports the use of a class PHP declares itself and
// Phego does not provide yet. instanceOf allows any name: the test is then
// by name, and false for every object a program can create.
func (c *compiler) checkBuiltinClass(name string, line int, instanceOf bool) {
	lower := phpname.Lower(name)
	if instanceOf || c.declaredClass[lower] || !isBuiltinClass(lower) || supportedBuiltinClasses[lower] {
		return
	}
	panic(&UnsupportedError{Feature: "builtin class " + name, File: c.unit.File, Line: positive(line, 1)})
}

// memberName is the name operand of a member access: the literal name, or
// the register its expression was evaluated into.
type memberName struct {
	operand uint32
	isReg   bool
}

func (fc *fnCompiler) memberName(n *ir.Node) memberName {
	if len(n.Kids) > 1 && n.Kids[1] != nil {
		return memberName{operand: fc.value(n.Kids[1]), isReg: true}
	}
	return memberName{operand: fc.c.stringConst(n.Text)}
}

func (m memberName) aux() uint16 {
	if m.isReg {
		return bytecode.MemberNameReg
	}
	return 0
}

// --- reads ---

// chainOp reports whether n is a member access whose object position
// continues a nullsafe chain.
func chainOp(n *ir.Node) bool {
	switch n.Op {
	case ir.OpProp, ir.OpNullsafe, ir.OpMethodCall, ir.OpNullsafeMC, ir.OpDim:
		return true
	}
	return false
}

// hasNullsafe reports whether the chain n heads short-circuits somewhere.
func hasNullsafe(n *ir.Node) bool {
	for n != nil && chainOp(n) {
		if n.Op == ir.OpNullsafe || n.Op == ir.OpNullsafeMC {
			return true
		}
		n = n.Kids[0]
	}
	return false
}

// shortCircuit is the result register and the exit of the nullsafe chain
// being compiled.
type shortCircuit struct {
	res uint32
	end int
}

// chain compiles a member access. The outermost access of a chain with a
// ?-> owns the chain's result: a null object anywhere in it makes the whole
// chain null without evaluating the rest (zend_short_circuiting).
func (fc *fnCompiler) chain(n *ir.Node, compile func() operand) operand {
	if fc.inBase || !hasNullsafe(n) {
		saved := fc.inBase
		fc.inBase = false
		o := compile()
		fc.inBase = saved
		return o
	}
	res := fc.alloc()
	saved := fc.short
	fc.short = &shortCircuit{res: res, end: fc.label()}
	r := fc.materialize(compile())
	fc.emit(bytecode.OpMove, 0, res, r, 0)
	fc.bind(fc.short.end)
	fc.short = saved
	fc.top = res - fc.base + 1
	return reg(res)
}

// baseValue compiles the object position of a member access. Its operands
// that are not themselves the object of a deeper access (arguments, names,
// keys) start chains of their own.
func (fc *fnCompiler) baseValue(n *ir.Node) uint32 {
	saved, short := fc.inBase, fc.short
	if chainOp(n) {
		fc.inBase = true
	} else {
		fc.inBase, fc.short = false, nil
	}
	r := fc.value(n)
	fc.inBase, fc.short = saved, short
	return r
}

// outside compiles an operand that is not part of the current chain.
func (fc *fnCompiler) outside(compile func()) {
	saved, short := fc.inBase, fc.short
	fc.inBase, fc.short = false, nil
	compile()
	fc.inBase, fc.short = saved, short
}

// nullsafe emits the short-circuit of ?->: a null object ends the chain.
func (fc *fnCompiler) nullsafe(obj uint32) {
	if fc.short == nil {
		panic(internalError{"nullsafe access outside a chain"})
	}
	mark := fc.top
	null := fc.alloc()
	fc.emit(bytecode.OpConst, 0, null, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	t := fc.alloc()
	fc.emit(bytecode.OpBinary, uint16(bytecode.Identical), t, obj, null)
	yes, no := fc.label(), fc.label()
	fc.branch(t, yes, no)
	fc.bind(yes)
	fc.emit(bytecode.OpMove, 0, fc.short.res, null, 0)
	fc.jump(fc.short.end)
	fc.bind(no)
	fc.top = mark
}

// propRead compiles $obj->name and $obj?->name as a value.
func (fc *fnCompiler) propRead(n *ir.Node) operand {
	return fc.chain(n, func() operand {
		obj := fc.baseValue(n.Kids[0])
		if n.Op == ir.OpNullsafe {
			fc.nullsafe(obj)
		}
		var name memberName
		fc.outside(func() { name = fc.memberName(n) })
		fc.at(n.Line)
		mode := bytecode.MemberRead
		if n.Flags&ir.FlagQuiet != 0 {
			mode = bytecode.MemberQuiet
		}
		res := fc.alloc()
		fc.emit(bytecode.OpPropGet, bytecode.MemberAux(0, name.isReg, mode, false), res, obj, name.operand)
		return reg(res)
	})
}

// staticPropRead compiles A::$name as a value.
func (fc *fnCompiler) staticPropRead(n *ir.Node) operand {
	ref, class := fc.classRef(n.Name, n.Kids[0])
	name := fc.memberName(n)
	fc.at(n.Line)
	mode := bytecode.MemberRead
	if n.Flags&ir.FlagQuiet != 0 {
		mode = bytecode.MemberQuiet
	}
	res := fc.alloc()
	fc.emit(bytecode.OpStaticGet, bytecode.MemberAux(ref, name.isReg, mode, false), res, class, name.operand)
	return reg(res)
}

func (fc *fnCompiler) classConst(n *ir.Node) operand {
	if n.Text == "" {
		fc.unsupported("class constant with a computed name", n.Line)
	}
	ref, class := fc.classRef(n.Name, n.Kids[0])
	fc.at(n.Line)
	res := fc.alloc()
	fc.emit(bytecode.OpClassConst, uint16(ref), res, class, fc.c.stringConst(n.Text))
	return reg(res)
}

func (fc *fnCompiler) className(n *ir.Node) operand {
	ref, class := fc.classRef(n.Name, n.Kids[0])
	fc.at(n.Line)
	res := fc.alloc()
	fc.emit(bytecode.OpClassName, uint16(ref), res, class, 0)
	return reg(res)
}

func (fc *fnCompiler) instanceOf(n *ir.Node) operand {
	v := fc.value(n.Kids[0])
	fc.instanceofOperand = true
	ref, class := fc.classRef(n.Name, n.Kids[1])
	fc.instanceofOperand = false
	fc.at(n.Line)
	res := fc.alloc()
	fc.emit(bytecode.OpInstanceof, uint16(ref), res, v, class)
	return reg(res)
}

func (fc *fnCompiler) clone(n *ir.Node) operand {
	operand := (*ir.Node)(nil)
	switch {
	case len(n.Kids) > 0:
		operand = n.Kids[0]
	case len(n.Args) == 1 && !n.Args[0].Spread && n.Args[0].Name == "":
		operand = n.Args[0].Value // clone($x), written as a call
	default:
		fc.unsupported("clone with arguments", n.Line)
	}
	v := fc.value(operand)
	fc.at(n.Line)
	res := fc.alloc()
	fc.emit(bytecode.OpClone, 0, res, v, 0)
	return reg(res)
}

func (fc *fnCompiler) this(n *ir.Node) operand {
	fc.at(n.Line)
	res := fc.alloc()
	fc.emit(bytecode.OpThis, 0, res, 0, 0)
	return reg(res)
}

// --- calls whose callee is found at run time ---

// isCall reports whether n is a function or method call, whose result PHP
// sends with ZEND_SEND_VAR_NO_REF_EX.
func isCall(n *ir.Node) bool {
	switch n.Op {
	case ir.OpCall, ir.OpCallValue, ir.OpMethodCall, ir.OpNullsafeMC, ir.OpStaticCall:
		return true
	}
	return false
}

// sendArgs sends the arguments of a call started by OpNew, OpInitMethod or
// OpInitStatic and makes the call. Each argument is sent as soon as it is
// evaluated, in a mode the callee decides when the instruction runs.
func (fc *fnCompiler) sendArgs(n *ir.Node, res uint32) {
	if fc.converting == n {
		// f(...): the started call becomes a Closure
		// (ZEND_CALLABLE_CONVERT).
		fc.converting = nil
		fc.sent() // the call's mark, which the conversion takes
		fc.pending--
		fc.emit(bytecode.OpCallableConvert, 0, res, 0, 0)
		return
	}
	entered := fc.pending
	fc.sent() // the call's mark
	fc.outside(func() {
		for _, a := range n.Args {
			mark := fc.top
			var name uint32
			if a.Name != "" {
				name = fc.c.stringConst(a.Name) + 1
			}
			switch v := a.Value; {
			case a.Spread:
				if v.Op == ir.OpVar {
					fc.c.line = v.Line
					fc.at(v.Line)
					fc.emit(bytecode.OpSendUnpack, 0, fc.varLoc(v.Var), 0, 0)
				} else {
					fc.emit(bytecode.OpSendUnpack, 0, fc.value(v), 0, 0)
				}
			case v.Op == ir.OpVar:
				fc.c.line = v.Line
				fc.at(v.Line)
				fc.emit(bytecode.OpPush, uint16(bytecode.PushAuto), fc.varLoc(v.Var), name, 0)
			case isCall(v):
				fc.emit(bytecode.OpPush, uint16(bytecode.PushResult), fc.value(v), name, 0)
			case v.Op == ir.OpDim && appendsElement(v):
				// PHP fetches $a[] for writing when the parameter is by
				// reference and throws "Cannot use [] for reading"
				// otherwise; there is no instruction for that throw yet.
				fc.unsupported("passing $a[] to a callee found at run time", positive(v.Line, n.Line))
			case (v.Op == ir.OpDim || v.Op == ir.OpProp || v.Op == ir.OpStaticProp) && name == 0 && !hasNullsafe(v):
				// ZEND_CHECK_FUNC_ARG: the callee decides whether the
				// element is fetched for writing and sent as a reference
				// (FETCH_DIM_FUNC_ARG) or read and sent as a value.
				c := fc.alloc()
				fc.emit(bytecode.OpArgByRef, 0, c, 0, 0)
				byRef, byValue, done := fc.label(), fc.label(), fc.label()
				fc.branch(c, byRef, byValue)
				line := fc.cur
				fc.bind(byRef)
				fc.emit(bytecode.OpPush, 0, fc.refArg(v, n.Line), 0, 0)
				fc.jump(done)
				fc.top, fc.cur = mark+1, line
				fc.bind(byValue)
				fc.emit(bytecode.OpPush, 0, fc.value(v), 0, 0)
				fc.bind(done)
			case v.Op == ir.OpDim || v.Op == ir.OpProp || v.Op == ir.OpStaticProp:
				fc.emit(bytecode.OpPush, uint16(bytecode.PushElement), fc.value(v), name, 0)
			default:
				fc.emit(bytecode.OpPush, uint16(bytecode.PushTemp), fc.value(v), name, 0)
			}
			fc.sent()
			fc.top = mark
		}
	})
	fc.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	after := fc.cur
	fc.at(n.Line)
	fc.emit(bytecode.OpCallPending, 0, res, 0, 0)
	fc.cur = after
	fc.pending = entered
}

func (fc *fnCompiler) newExpr(n *ir.Node) operand {
	if n.Class != nil {
		fc.unsupported("anonymous class", n.Line)
	}
	res := fc.alloc()
	ref, class := fc.classRef(n.Name, n.Kids[0])
	fc.at(n.Line)
	none := fc.label()
	aux := uint16(ref)
	if len(n.Args) > 0 {
		aux |= bytecode.NewArgs
	}
	pc := fc.emit(bytecode.OpNew, aux, res, class, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 2, none})
	// The constructor's result is discarded.
	t := fc.alloc()
	fc.sendArgs(n, t)
	fc.emit(bytecode.OpDrop, 0, t, 0, 0)
	fc.bind(none)
	fc.top = res - fc.base + 1
	return reg(res)
}

func (fc *fnCompiler) methodCall(n *ir.Node) operand {
	return fc.chain(n, func() operand {
		res := fc.alloc()
		obj := fc.baseValue(n.Kids[0])
		if n.Op == ir.OpNullsafeMC {
			fc.nullsafe(obj)
		}
		var name memberName
		fc.outside(func() { name = fc.memberName(n) })
		fc.at(n.Line)
		fc.emit(bytecode.OpInitMethod, name.aux(), obj, name.operand, 0)
		fc.sendArgs(n, res)
		fc.top = res - fc.base + 1
		return reg(res)
	})
}

func (fc *fnCompiler) staticCall(n *ir.Node) operand {
	res := fc.alloc()
	ref, class := fc.classRef(n.Name, n.Kids[0])
	name := fc.memberName(n)
	fc.at(n.Line)
	fc.emit(bytecode.OpInitStatic, uint16(ref)|name.aux(), class, name.operand, 0)
	fc.sendArgs(n, res)
	fc.top = res - fc.base + 1
	return reg(res)
}

// --- write targets ---

// delayed is the object or container a write goes through, with its
// operand expressions already evaluated and its fetch still to emit, as
// zend_delayed_compile_var leaves it: PHP evaluates every operand of a
// target, then the assigned value, and only then fetches the target's
// levels.
type delayed struct {
	// fetch emits the fetch for writing. It returns a Value register that
	// holds the object, or the element register (element true) that names
	// the place it is in.
	fetch func(mode bytecode.MemberMode) (r uint32, element bool)
}

// delay compiles the operands of n, the base of a property or element
// write.
func (fc *fnCompiler) delay(n *ir.Node) delayed {
	switch n.Op {
	case ir.OpVar:
		line := n.Line
		return delayed{fetch: func(bytecode.MemberMode) (uint32, bool) {
			fc.at(line)
			r := fc.alloc()
			fc.emit(bytecode.OpLoad, 0, r, fc.varLoc(n.Var), 0)
			return r, false
		}}
	case ir.OpThis:
		return delayed{fetch: func(bytecode.MemberMode) (uint32, bool) {
			r := fc.alloc()
			fc.emit(bytecode.OpThis, 0, r, 0, 0)
			return r, false
		}}
	case ir.OpProp:
		inner := fc.delay(n.Kids[0])
		name := fc.memberName(n)
		line := fc.cur
		return delayed{fetch: func(mode bytecode.MemberMode) (uint32, bool) {
			obj, element := inner.fetch(bytecode.MemberWrite)
			fc.cur = line
			if element {
				obj = elemReg
			}
			fc.emit(bytecode.OpFetchProp, bytecode.MemberAux(0, name.isReg, mode, element), elemReg, obj, name.operand)
			return elemReg, true
		}}
	case ir.OpStaticProp:
		ref, class := fc.classRef(n.Name, n.Kids[0])
		name := fc.memberName(n)
		line := fc.cur
		return delayed{fetch: func(mode bytecode.MemberMode) (uint32, bool) {
			fc.cur = line
			if mode == bytecode.MemberUnset {
				mode = bytecode.MemberWrite
			}
			fc.emit(bytecode.OpFetchStatic, bytecode.MemberAux(ref, name.isReg, mode, false), elemReg, class, name.operand)
			return elemReg, true
		}}
	case ir.OpDim:
		// The keys of an element below a property are evaluated into
		// registers: no variable may be read inside the fetch sequence.
		var key uint32
		hasKey := n.Kids[1] != nil
		if n.Kids[0].Op == ir.OpVar {
			base := n.Kids[0]
			if hasKey {
				key = fc.value(n.Kids[1])
			}
			line := fc.cur
			return delayed{fetch: func(mode bytecode.MemberMode) (uint32, bool) {
				fc.cur = line
				flags := bytecode.FetchFlags(mode)
				if hasKey {
					fc.emit(bytecode.OpFetchDim, uint16(flags), elemReg, fc.varLoc(base.Var), key)
				} else {
					fc.emit(bytecode.OpFetchDim, uint16(flags|bytecode.FetchAppend), elemReg, fc.varLoc(base.Var), 0)
				}
				return elemReg, true
			}}
		}
		inner := fc.delay(n.Kids[0])
		if hasKey {
			key = fc.value(n.Kids[1])
		}
		line := fc.cur
		return delayed{fetch: func(mode bytecode.MemberMode) (uint32, bool) {
			_, element := inner.fetch(bytecode.MemberWrite)
			if !element {
				fc.unsupported("array write through a non-variable", line)
			}
			fc.cur = line
			flags := bytecode.FetchFlags(mode)
			if hasKey {
				fc.emit(bytecode.OpFetchDim, uint16(flags), elemReg, elemReg, key)
			} else {
				fc.emit(bytecode.OpFetchDim, uint16(flags|bytecode.FetchAppend), elemReg, elemReg, 0)
			}
			return elemReg, true
		}}
	case ir.OpNullsafe, ir.OpNullsafeMC:
		fc.unsupported("nullsafe operator in a write context", n.Line)
	}
	// Any other expression (a call, new) yields the object itself.
	r := fc.value(n)
	return delayed{fetch: func(bytecode.MemberMode) (uint32, bool) { return r, false }}
}

// propTarget is a property write target with its operands evaluated.
type propTarget struct {
	obj  delayed
	name memberName
	line int
}

func (fc *fnCompiler) propTarget(n *ir.Node) propTarget {
	if n.Op != ir.OpProp {
		fc.unsupported("nullsafe operator in a write context", n.Line)
	}
	obj := fc.delay(n.Kids[0])
	name := fc.memberName(n)
	fc.at(n.Line)
	return propTarget{obj: obj, name: name, line: fc.cur}
}

// object emits the fetch of the target's object and returns its operand
// with the Aux bit that says where it is.
func (fc *fnCompiler) object(t propTarget, mode bytecode.MemberMode) (r uint32, aux uint16) {
	r, element := t.obj.fetch(mode)
	fc.cur = t.line
	if element {
		return elemReg, bytecode.MemberObjLocation
	}
	return r, 0
}

func resultAux(used bool) uint16 {
	if used {
		return bytecode.MemberResult
	}
	return 0
}

// assignProp compiles "$obj->name = value" (ZEND_ASSIGN_OBJ).
func (fc *fnCompiler) assignProp(n *ir.Node, used bool) operand {
	t := fc.propTarget(n.Kids[0])
	v := fc.value(n.Kids[1])
	after := fc.cur
	rv := fc.ownCopy(v, n.Kids[1], used)
	obj, aux := fc.object(t, bytecode.MemberWrite)
	fc.emit(bytecode.OpPropSet, aux|t.name.aux()|resultAux(used), obj, t.name.operand, rv)
	fc.cur = after
	return reg(rv)
}

// ownCopy returns a register the assignment may replace with its result: a
// fresh copy when the value's register belongs to something else.
func (fc *fnCompiler) ownCopy(v uint32, n *ir.Node, used bool) uint32 {
	if !used || v >= fc.base {
		return v
	}
	r := fc.alloc()
	fc.emit(bytecode.OpMove, 0, r, v, 0)
	return r
}

// assignStatic compiles "A::$name = value" (ZEND_ASSIGN_STATIC_PROP).
func (fc *fnCompiler) assignStatic(n *ir.Node, used bool) operand {
	target := n.Kids[0]
	ref, class := fc.classRef(target.Name, target.Kids[0])
	name := fc.memberName(target)
	v := fc.value(n.Kids[1])
	after := fc.cur
	rv := fc.ownCopy(v, n.Kids[1], used)
	fc.at(target.Line)
	fc.emit(bytecode.OpStaticSet, bytecode.MemberAux(ref, name.isReg, 0, false)|resultAux(used), rv, class, name.operand)
	fc.cur = after
	return reg(rv)
}

// propOp compiles "$obj->name op= value" and "$obj->name ??= value"
// without the lazy part: read, combine, write, on one fetched object.
func (fc *fnCompiler) assignOpProp(n *ir.Node, op bytecode.BinaryOp) operand {
	t := fc.propTarget(n.Kids[0])
	v := fc.value(n.Kids[1])
	after := fc.cur
	obj, aux := fc.object(t, bytecode.MemberWrite)
	old := fc.alloc()
	fc.emit(bytecode.OpPropGet, aux|t.name.aux()|uint16(bytecode.MemberAssignOp)<<4, old, obj, t.name.operand)
	res := fc.alloc()
	fc.emit(bytecode.OpBinary, uint16(op), res, old, v)
	fc.emit(bytecode.OpPropSet, aux|t.name.aux()|bytecode.MemberResult, obj, t.name.operand, res)
	fc.cur = after
	return reg(res)
}

func (fc *fnCompiler) assignOpStatic(n *ir.Node, op bytecode.BinaryOp) operand {
	target := n.Kids[0]
	ref, class := fc.classRef(target.Name, target.Kids[0])
	name := fc.memberName(target)
	v := fc.value(n.Kids[1])
	after := fc.cur
	fc.at(target.Line)
	old := fc.alloc()
	fc.emit(bytecode.OpStaticGet, bytecode.MemberAux(ref, name.isReg, bytecode.MemberRead, false), old, class, name.operand)
	res := fc.alloc()
	fc.emit(bytecode.OpBinary, uint16(op), res, old, v)
	fc.emit(bytecode.OpStaticSet, bytecode.MemberAux(ref, name.isReg, 0, false)|bytecode.MemberResult, res, class, name.operand)
	fc.cur = after
	return reg(res)
}

// incDecProp compiles ++ and -- on a property: read, step, write
// (ZEND_PRE_INC_OBJ and its siblings).
func (fc *fnCompiler) incDecProp(n *ir.Node) operand {
	target := n.Kids[0]
	step, stepMode := bytecode.Increment, uint16(bytecode.MemberIncrement)<<4
	if n.Text == "--" {
		step, stepMode = bytecode.Decrement, uint16(bytecode.MemberDecrement)<<4
	}
	var old, next uint32
	if target.Op == ir.OpStaticProp {
		ref, class := fc.classRef(target.Name, target.Kids[0])
		name := fc.memberName(target)
		fc.at(target.Line)
		old, next = fc.alloc(), fc.alloc()
		fc.emit(bytecode.OpStaticGet, bytecode.MemberAux(ref, name.isReg, bytecode.MemberRead, false), old, class, name.operand)
		fc.emit(bytecode.OpUnary, uint16(step), next, old, 0)
		fc.emit(bytecode.OpStaticSet, bytecode.MemberAux(ref, name.isReg, 0, false)|bytecode.MemberResult|stepMode, next, class, name.operand)
	} else {
		t := fc.propTarget(target)
		obj, aux := fc.object(t, bytecode.MemberWrite)
		old, next = fc.alloc(), fc.alloc()
		fc.emit(bytecode.OpPropGet, aux|t.name.aux()|uint16(bytecode.MemberIncDec)<<4, old, obj, t.name.operand)
		fc.emit(bytecode.OpUnary, uint16(step), next, old, 0)
		fc.emit(bytecode.OpPropSet, aux|t.name.aux()|bytecode.MemberResult|stepMode, obj, t.name.operand, next)
	}
	if n.Op == ir.OpPostInc {
		return reg(old)
	}
	return reg(next)
}

// coalesceSetMember compiles "$obj->name ??= value" and "A::$name ??=
// value": the target is read quietly, and only when it is null is the
// value evaluated and assigned.
func (fc *fnCompiler) coalesceSetMember(n *ir.Node) operand {
	target := n.Kids[0]
	res := fc.alloc()
	mark := fc.top
	assign, end := fc.label(), fc.label()
	test := func(cur uint32) {
		null, t := fc.alloc(), fc.alloc()
		fc.emit(bytecode.OpConst, 0, null, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
		fc.emit(bytecode.OpBinary, uint16(bytecode.Identical), t, cur, null)
		keep := fc.label()
		fc.branch(t, assign, keep)
		fc.bind(keep)
		fc.emit(bytecode.OpMove, 0, res, cur, 0)
		fc.jump(end)
		fc.bind(assign)
	}
	if target.Op == ir.OpStaticProp {
		ref, class := fc.classRef(target.Name, target.Kids[0])
		name := fc.memberName(target)
		fc.at(target.Line)
		cur := fc.alloc()
		fc.emit(bytecode.OpStaticGet, bytecode.MemberAux(ref, name.isReg, bytecode.MemberQuiet, false), cur, class, name.operand)
		test(cur)
		v := fc.value(n.Kids[1])
		fc.emit(bytecode.OpMove, 0, res, v, 0)
		fc.emit(bytecode.OpStaticSet, bytecode.MemberAux(ref, name.isReg, 0, false)|bytecode.MemberResult, res, class, name.operand)
	} else {
		// The object is read once for the test; the assignment fetches the
		// target again for writing, as PHP compiles the target twice.
		obj := fc.value(target.Kids[0])
		name := fc.memberName(target)
		fc.at(target.Line)
		cur := fc.alloc()
		fc.emit(bytecode.OpPropGet, bytecode.MemberAux(0, name.isReg, bytecode.MemberQuiet, false), cur, obj, name.operand)
		test(cur)
		v := fc.value(n.Kids[1])
		fc.emit(bytecode.OpMove, 0, res, v, 0)
		fc.emit(bytecode.OpPropSet, name.aux()|bytecode.MemberResult, obj, name.operand, res)
	}
	fc.bind(end)
	fc.top = mark
	return reg(res)
}

// unsetProp compiles unset($obj->name) (ZEND_UNSET_OBJ).
func (fc *fnCompiler) unsetProp(n *ir.Node) {
	t := fc.propTarget(n)
	obj, aux := fc.object(t, bytecode.MemberUnset)
	fc.emit(bytecode.OpPropUnset, aux|t.name.aux(), obj, t.name.operand, 0)
}

// issetMember compiles the isset or empty test of a property or a static
// property into a bool register.
func (fc *fnCompiler) issetMember(k *ir.Node, empty bool) uint32 {
	mode := bytecode.MemberIsset
	if empty {
		mode = bytecode.MemberEmpty
	}
	if k.Op == ir.OpStaticProp {
		ref, class := fc.classRef(k.Name, k.Kids[0])
		name := fc.memberName(k)
		fc.at(k.Line)
		r := fc.alloc()
		fc.emit(bytecode.OpStaticGet, bytecode.MemberAux(ref, name.isReg, mode, false), r, class, name.operand)
		return r
	}
	var r uint32
	o := fc.chain(k, func() operand {
		obj := fc.baseValue(k.Kids[0])
		if k.Op == ir.OpNullsafe {
			fc.nullsafe(obj)
		}
		var name memberName
		fc.outside(func() { name = fc.memberName(k) })
		fc.at(k.Line)
		r = fc.alloc()
		fc.emit(bytecode.OpPropIsset, bytecode.MemberAux(0, name.isReg, mode, false), r, obj, name.operand)
		return reg(r)
	})
	if o.reg != r {
		// The chain short-circuited to null: isset is false, empty true.
		res := fc.alloc()
		null, t := fc.alloc(), fc.alloc()
		fc.emit(bytecode.OpConst, 0, null, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
		fc.emit(bytecode.OpBinary, uint16(bytecode.Identical), t, o.reg, null)
		yes, no, end := fc.label(), fc.label(), fc.label()
		fc.branch(t, yes, no)
		fc.bind(yes)
		fc.boolConst(res, empty)
		fc.jump(end)
		fc.bind(no)
		fc.emit(bytecode.OpMove, 0, res, o.reg, 0)
		fc.bind(end)
		return res
	}
	return r
}

// memberRef compiles a reference to a property or a static property into a
// Value register (the property side of ZEND_ASSIGN_OBJ_REF, ZEND_SEND_REF
// and FE_RESET_RW).
func (fc *fnCompiler) memberRef(n *ir.Node) uint32 {
	if n.Op == ir.OpStaticProp {
		ref, class := fc.classRef(n.Name, n.Kids[0])
		name := fc.memberName(n)
		fc.at(n.Line)
		r := fc.alloc()
		fc.emit(bytecode.OpStaticRef, bytecode.MemberAux(ref, name.isReg, 0, false), r, class, name.operand)
		return r
	}
	obj := fc.value(n.Kids[0])
	name := fc.memberName(n)
	fc.at(n.Line)
	r := fc.alloc()
	fc.emit(bytecode.OpPropRef, name.aux(), r, obj, name.operand)
	return r
}

// --- declarations ---

// classDecl compiles a class statement: a class PHP did not bind while
// compiling is declared when the statement runs.
func (fc *fnCompiler) classDecl(n *ir.Node) {
	cl := n.Class
	index, ok := fc.c.classIndex[cl]
	if !ok {
		if ue := fc.c.classUnsupported(cl); ue != nil {
			panic(ue)
		}
		panic(internalError{"class " + cl.Name + " is not in the class table"})
	}
	if cl.Hoisted {
		return
	}
	fc.at(n.Line)
	fc.emit(bytecode.OpDeclareClass, 0, 0, index, 0)
}

// promote emits the assignments of the promoted constructor parameters,
// after every parameter is received.
func (fc *fnCompiler) promote() {
	for _, p := range fc.f.Params {
		if len(p.Promoted) == 0 {
			continue
		}
		mark := fc.top
		fc.cur = positive(p.Line, fc.f.Line)
		this, v := fc.alloc(), fc.alloc()
		fc.emit(bytecode.OpThis, 0, this, 0, 0)
		fc.emit(bytecode.OpLoad, 0, v, fc.varLoc(p.Var), 0)
		fc.emit(bytecode.OpPropSet, 0, this, fc.c.stringConst(p.Name), v)
		fc.top = mark
	}
}

// returnType reports the declared return type and how a return is checked.
func (fc *fnCompiler) returnType() string {
	if fc.f.Return == nil {
		return ""
	}
	return fc.f.Return.Zend()
}

// lowerName folds a PHP identifier for comparison.
func lowerName(s string) string { return phpname.Lower(s) }

// callValue compiles $callable(args) for positional arguments
// (ZEND_INIT_DYNAMIC_CALL): an object is called through __invoke, a string
// names a function.
func (fc *fnCompiler) callValue(n *ir.Node) operand {
	res := fc.alloc()
	callee := fc.value(n.Kids[0])
	fc.at(n.Line)
	fc.emit(bytecode.OpInitMethod, bytecode.InitCallable, callee, 0, 0)
	fc.sendArgs(n, res)
	fc.top = res - fc.base + 1
	return reg(res)
}

// bindMember compiles "$obj->name =& source" and "A::$name =& source"
// (ZEND_ASSIGN_OBJ_REF, ZEND_ASSIGN_STATIC_PROP_REF): the property's place
// is fetched for writing and bound to the source's reference.
func (fc *fnCompiler) bindMember(n *ir.Node, used bool) operand {
	target, source := n.Kids[0], n.Kids[1]
	if used {
		fc.unsupported("value of an assignment by reference to a property", n.Line)
	}
	source2ref := func() uint32 {
		switch source.Op {
		case ir.OpProp, ir.OpStaticProp:
			return fc.memberRef(source)
		}
		src := fc.refSource(source, n.Line)
		ref := fc.alloc()
		fc.emit(bytecode.OpMakeRef, 0, ref, src, 0)
		return ref
	}
	bind := uint16(bytecode.MemberBindRef) << 4
	if target.Op == ir.OpStaticProp {
		ref, class := fc.classRef(target.Name, target.Kids[0])
		name := fc.memberName(target)
		r := source2ref()
		fc.at(target.Line)
		fc.emit(bytecode.OpStaticSet, bytecode.MemberAux(ref, name.isReg, 0, false)|bind, r, class, name.operand)
		return reg(fc.alloc())
	}
	t := fc.propTarget(target)
	r := source2ref()
	obj, aux := fc.object(t, bytecode.MemberWrite)
	fc.emit(bytecode.OpPropSet, aux|t.name.aux()|bind, obj, t.name.operand, r)
	return reg(fc.alloc())
}

// memberBased reports whether the element target n is reached through a
// property or a static property.
func memberBased(n *ir.Node) bool {
	for n != nil && n.Op == ir.OpDim {
		n = n.Kids[0]
	}
	return n != nil && (n.Op == ir.OpProp || n.Op == ir.OpStaticProp)
}

// pureTarget reports whether evaluating the operands of the write target n
// twice is the same as evaluating them once: variables, $this and literal
// names and keys only.
func pureTarget(n *ir.Node) bool {
	for n != nil {
		switch n.Op {
		case ir.OpVar, ir.OpThis:
			return true
		case ir.OpDim:
			if k := n.Kids[1]; k != nil && k.Op != ir.OpLiteral && k.Op != ir.OpVar {
				return false
			}
		case ir.OpProp:
			if len(n.Kids) > 1 && n.Kids[1] != nil {
				return false
			}
		case ir.OpStaticProp:
			return n.Name != nil && (len(n.Kids) < 2 || n.Kids[1] == nil)
		default:
			return false
		}
		n = n.Kids[0]
	}
	return false
}

// quietRead is the target n as a quiet read (FETCH_*_IS at every level).
func quietRead(n *ir.Node) *ir.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Flags = ir.FlagQuiet
	switch n.Op {
	case ir.OpDim, ir.OpProp:
		c.Kids = append([]*ir.Node(nil), n.Kids...)
		c.Kids[0] = quietRead(n.Kids[0])
	}
	return &c
}

// coalesceSetMemberDim compiles "$obj->prop[k] ??= value": the element is
// read quietly, and only when it is null is the value evaluated and the
// element written. PHP compiles the target twice as well, sharing its
// operand expressions, which are therefore limited here to ones without
// side effects.
func (fc *fnCompiler) coalesceSetMemberDim(n *ir.Node) operand {
	target := n.Kids[0]
	if !pureTarget(target) {
		fc.unsupported("??= on an element of a property with computed operands", n.Line)
	}
	res := fc.alloc()
	mark := fc.top
	cur := fc.value(quietRead(target))
	null, t := fc.alloc(), fc.alloc()
	fc.emit(bytecode.OpConst, 0, null, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	fc.emit(bytecode.OpBinary, uint16(bytecode.Identical), t, cur, null)
	assign, keep, end := fc.label(), fc.label(), fc.label()
	fc.branch(t, assign, keep)
	fc.bind(keep)
	fc.emit(bytecode.OpMove, 0, res, cur, 0)
	fc.jump(end)
	fc.bind(assign)
	fc.top = mark
	dt := fc.dimTarget(target, n.Line)
	v := fc.value(n.Kids[1])
	fc.emit(bytecode.OpMove, 0, res, v, 0)
	fc.writeDim(dt, reg(v), false, false)
	fc.bind(end)
	fc.top = mark
	return reg(res)
}
