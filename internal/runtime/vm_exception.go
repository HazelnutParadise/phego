package runtime

import (
	"errors"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 7C: REQ-001, REQ-016, REQ-025, REQ-026. The built-in classes
// that implement Throwable (zend_exceptions.c, spl_exceptions.c) and the
// PHP exception objects the VM makes of the runtime's Go errors.
//
// The class descriptions are immutable and shared between requests. An
// exception object, its trace and the values the trace holds belong to the
// run that created them.

//go:generate sh -c "docker run --rm --network none --cap-drop ALL -v \"$PWD/gen\":/g:ro php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 php -n /g/throwables.php > throwables_gen.go && gofmt -w throwables_gen.go"

// The methods of Exception, Error and ErrorException the VM implements
// (methodSpec.native), after the methods of Closure.
const (
	excConstruct uint8 = closureMagic + 1 + iota
	excWakeup
	excGetMessage
	excGetCode
	excGetFile
	excGetLine
	excGetTrace
	excGetPrevious
	excGetTraceAsString
	excToString
	excErrorConstruct
	excGetSeverity
)

// excParam is one parameter of a native method: its name, its type, and
// whether null stands for "not given" (a ?type parameter).
type excParam struct {
	name, typ string
	nullable  bool
}

// excNative describes a native method of the exception classes.
type excNative struct {
	name   string
	params []excParam
}

var excNatives = map[uint8]excNative{
	excConstruct: {"__construct", []excParam{{"message", "string", false}, {"code", "int", false}, {"previous", "Throwable", true}}},
	excErrorConstruct: {"__construct", []excParam{{"message", "string", false}, {"code", "int", false}, {"severity", "int", false},
		{"filename", "string", true}, {"line", "int", true}, {"previous", "Throwable", true}}},
	excWakeup:           {"__wakeup", nil},
	excGetMessage:       {"getMessage", nil},
	excGetCode:          {"getCode", nil},
	excGetFile:          {"getFile", nil},
	excGetLine:          {"getLine", nil},
	excGetTrace:         {"getTrace", nil},
	excGetPrevious:      {"getPrevious", nil},
	excGetTraceAsString: {"getTraceAsString", nil},
	excToString:         {"__toString", nil},
	excGetSeverity:      {"getSeverity", nil},
}

// throwableBuiltin is PHP's Throwable interface, which extends Stringable.
var throwableBuiltin = &class{name: "Throwable", schema: &classSchema{classSpec: classSpec{name: "Throwable", isInterface: true, interfaces: []*class{stringableBuiltin}}}}

// exceptionBuiltin and errorBuiltin are the two roots; throwableBuiltins
// holds every built-in class that implements Throwable, and the interface,
// by lower-cased name.
var exceptionBuiltin, errorBuiltin *class
var throwableBuiltins = func() map[string]*class {
	m := func(name string, kind uint8, final bool) methodSpec {
		// The VM calls the method through its native kind; the runtime's
		// own calls (a string conversion's __toString) come through call.
		call := func(h *heap, this value, _ []value) (value, error) {
			if h.vm == nil || this.kind != valueObject || !isThrowableClass(this.object.class) {
				return value{}, errors.New("a method of an exception class was called outside the VM")
			}
			out, err := h.vm.excResult(kind, this.object)
			if err == nil && out.kind >= valueArray {
				retain(out)
			}
			return out, err
		}
		return methodSpec{name: name, visibility: publicAccess, final: final, native: kind, call: call}
	}
	root := func(name string) classSpec {
		return classSpec{name: name, interfaces: []*class{throwableBuiltin}, noDynamic: false,
			properties: []propertySpec{
				{name: "message", visibility: protectedAccess, hasDefault: true, initial: []byte("")},
				{name: "string", typeName: "string", visibility: privateAccess, hasDefault: true, initial: []byte("")},
				{name: "code", visibility: protectedAccess, hasDefault: true, initial: int64(0)},
				{name: "file", typeName: "string", visibility: protectedAccess, hasDefault: true, initial: []byte("")},
				{name: "line", typeName: "int", visibility: protectedAccess},
				{name: "trace", typeName: "array", visibility: privateAccess, hasDefault: true, initial: []SnapshotEntry{}},
				{name: "previous", typeName: "?Throwable", visibility: privateAccess, hasDefault: true, initial: nil},
			},
			methods: []methodSpec{
				m("__construct", excConstruct, false), m("__wakeup", excWakeup, false),
				m("getMessage", excGetMessage, true), m("getCode", excGetCode, true), m("getFile", excGetFile, true),
				m("getLine", excGetLine, true), m("getTrace", excGetTrace, true), m("getPrevious", excGetPrevious, true),
				m("getTraceAsString", excGetTraceAsString, true), m("__toString", excToString, false),
			}}
	}
	out := map[string]*class{"throwable": throwableBuiltin}
	for _, d := range throwableDecls {
		var spec classSpec
		switch d.name {
		case "Exception", "Error":
			spec = root(d.name)
		case "ErrorException":
			spec = classSpec{name: d.name,
				properties: []propertySpec{{name: "severity", typeName: "int", visibility: protectedAccess, hasDefault: true, initial: int64(1)}},
				methods:    []methodSpec{m("__construct", excErrorConstruct, false), m("getSeverity", excGetSeverity, true)}}
		default:
			spec = classSpec{name: d.name}
		}
		spec.final = d.final
		if d.parent != "" {
			spec.parent = out[phpname.Lower(d.parent)]
			if spec.parent == nil {
				panic("throwable " + d.name + " is listed before its parent")
			}
		}
		c, err := buildClass(spec)
		if err != nil {
			panic(err)
		}
		out[phpname.Lower(d.name)] = c
	}
	exceptionBuiltin, errorBuiltin = out["exception"], out["error"]
	return out
}()

// linkThrowable checks a class declaration against the built-in exception
// classes: only Exception and Error implement Throwable themselves
// (zend_implement_throwable), and a property of theirs is redeclared with
// the same type (zend_do_inheritance's property check, which the lowering
// makes for parents the program declares).
func (s *vmState) linkThrowable(d *vmClassDecl, spec *classSpec) error {
	if !s.throwables {
		return nil
	}
	if !spec.isInterface && (spec.parent == nil || !isThrowableClass(spec.parent)) {
		for _, i := range spec.interfaces {
			if classIs(i, throwableBuiltin) {
				return &vmFatal{message: "Class " + d.name + " cannot implement interface Throwable, extend Exception or Error instead", line: d.line}
			}
		}
	}
	if spec.parent == nil || !isThrowableClass(spec.parent) {
		return nil
	}
	for i := range d.props {
		p := &d.props[i]
		base, err := resolveProperty(spec.parent, p.name, spec.parent)
		if err != nil || base == nil || base.owner == nil || base.owner.vm != nil || base.visibility == privateAccess {
			continue
		}
		switch {
		case base.typeName == p.typ:
		case base.typeName == "":
			return &vmFatal{message: "Type of " + d.name + "::$" + p.name + " must be omitted to match the parent definition in class " + base.owner.name, line: d.line}
		default:
			return &vmFatal{message: "Type of " + d.name + "::$" + p.name + " must be " + base.typeName + " (as in class " + base.owner.name + ")", line: d.line}
		}
	}
	return nil
}

// isThrowableClass reports whether c implements Throwable.
func isThrowableClass(c *class) bool {
	return c != nil && (classIs(c, exceptionBuiltin) || classIs(c, errorBuiltin))
}

// excBase is i_get_exception_base: the root class whose private
// properties an exception object has.
func excBase(c *class) *class {
	if classIs(c, errorBuiltin) {
		return errorBuiltin
	}
	return exceptionBuiltin
}

// excSlot is the property name of exception o, as its root class sees it.
func excSlot(o *object, name string) *value {
	p, err := resolveProperty(o.class, name, excBase(o.class))
	if err != nil || p == nil {
		return nil
	}
	slot, ok := o.props.lookup(p.key)
	if !ok {
		return nil
	}
	return slot
}

// excGet reads property name of exception o: a borrowed value, null when
// the property is unset.
func excGet(o *object, name string) value {
	slot := excSlot(o, name)
	if slot == nil {
		return nullValue()
	}
	v := slot.deref()
	if v.kind == valueUndefined {
		return nullValue()
	}
	return v
}

// excSet assigns v to property name of exception o.
func (s *vmState) excSet(o *object, name string, v value) error {
	slot := excSlot(o, name)
	if slot == nil {
		return s.internal("exception class %s has no property %s", o.class.name, name)
	}
	return s.h.assign(slot, v)
}

// vmThrown is a PHP exception object on its way through Go code: the
// error an instruction returns when PHP code it ran left an exception, and
// what a nested run ends with. It owns one reference to the object until
// thrownObject takes it.
type vmThrown struct{ object *object }

func (e *vmThrown) Error() string {
	if e.object == nil {
		return "PHP exception"
	}
	msg, _ := e.object.owner.toString(excGet(e.object, "message"))
	return e.object.class.name + ": " + msg
}

// initException is zend_default_exception_new: an object of a class that
// implements Throwable records where it was created and the backtrace
// there, whether or not a constructor runs. internal names an internal
// function whose frame the backtrace starts with.
func (s *vmState) initException(o *object, internal string, args []value) error {
	_, _, file, line := s.location()
	trace := s.traceArray(s.traceFrames(internal, args))
	if err := s.excSet(o, "file", stringValue(vmFileName(file))); err != nil {
		return err
	}
	lv, err := intValue(int64(line), s.prog.width)
	if err != nil {
		return err
	}
	if err := s.excSet(o, "line", lv); err != nil {
		return err
	}
	err = s.excSet(o, "trace", trace)
	if rerr := s.h.release(trace); err == nil {
		err = rerr
	}
	return err
}

// throwableClass finds a built-in or declared class that implements
// Throwable by name.
func (s *vmState) throwableClass(name string) *class {
	c := s.lookupClass(name)
	if !isThrowableClass(c) {
		return nil
	}
	return c
}

// newThrowable creates the exception object of a throwable the runtime
// raised (zend_throw_exception): an owned object of class with message,
// located at the current instruction.
func (s *vmState) newThrowable(class, message, internal string, args []value) (*object, error) {
	c := s.throwableClass(class)
	if c == nil {
		return nil, s.internal("the runtime raised %s, which is not a throwable class", class)
	}
	v, err := s.h.instantiate(c)
	if err != nil {
		return nil, err
	}
	retain(v)
	if err := s.initException(v.object, internal, args); err != nil {
		_ = s.h.release(v)
		return nil, err
	}
	if message != "" {
		if err := s.excSet(v.object, "message", stringValue(message)); err != nil {
			_ = s.h.release(v)
			return nil, err
		}
	}
	return v.object, nil
}

// thrownObject turns the error of an instruction into the PHP exception
// object it stands for, owned by the caller. ok is false for an error PHP
// code cannot catch (a fatal error, exit, a host stop, an internal error);
// err is then what ends the run.
func (s *vmState) thrownObject(err error) (ex *object, ok bool, fail error) {
	var fatal *vmFatal
	if errors.As(err, &fatal) {
		return nil, false, err
	}
	switch e := err.(type) {
	case *vmThrown:
		o := e.object
		e.object = nil
		if o == nil {
			return nil, false, s.internal("an exception object was thrown twice")
		}
		return o, true, nil
	case *Throwable:
		o, cerr := s.newThrowable(e.Class, e.Message, "", nil)
		if cerr != nil {
			return nil, false, cerr
		}
		return o, true, nil
	case *vmInternalFrameThrow:
		o, cerr := s.newThrowable(e.thrown.Class, e.thrown.Message, e.function, e.args)
		if cerr != nil {
			return nil, false, cerr
		}
		return o, true, nil
	case completion:
		if e.kind != endThrow || e.err == nil {
			return nil, false, err
		}
		o, ok, fail := s.thrownObject(e.err)
		if !ok {
			return nil, false, fail
		}
		if e.previous != nil {
			prev, pok, pfail := s.thrownChain(e.previous)
			if pfail != nil && !pok {
				_ = s.releaseObject(o)
				return nil, false, pfail
			}
			if prev != nil {
				s.setPrevious(o, prev)
			}
		}
		return o, true, nil
	}
	if errors.Is(err, ErrNextElementOccupied) {
		o, cerr := s.newThrowable("Error", err.Error(), "", nil)
		if cerr != nil {
			return nil, false, cerr
		}
		return o, true, nil
	}
	// A wrapped error: the first of these it carries.
	var thrown *vmThrown
	var internal *vmInternalFrameThrow
	var c completion
	var t *Throwable
	switch {
	case errors.As(err, &thrown):
		return s.thrownObject(thrown)
	case errors.As(err, &c) && c.kind != endThrow:
		return nil, false, err
	case errors.As(err, &c):
		return s.thrownObject(c)
	case errors.As(err, &internal):
		return s.thrownObject(internal)
	case errors.As(err, &t):
		return s.thrownObject(t)
	}
	return nil, false, err
}

// thrownChain materializes the previous errors of a completion, oldest
// link last, as one chain: each later one becomes the previous of the one
// before it.
func (s *vmState) thrownChain(err error) (*object, bool, error) {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		if _, isCompletion := err.(completion); !isCompletion {
			var head *object
			for _, child := range multi.Unwrap() {
				if child == nil {
					continue
				}
				o, ok, fail := s.thrownChain(child)
				if !ok {
					if head != nil {
						_ = s.releaseObject(head)
					}
					return nil, false, fail
				}
				if head == nil {
					head = o
				} else if o != nil {
					s.setPrevious(head, o)
				}
			}
			return head, true, nil
		}
	}
	if t, ok := err.(*vmThrown); ok && t.object == nil {
		// An exception the VM already threw and handled: the execution
		// still listed it as the event before this one.
		return nil, true, nil
	}
	if c, ok := err.(completion); ok {
		if t, ok := c.err.(*vmThrown); ok && t.object == nil {
			return nil, true, nil
		}
	}
	return s.thrownObject(err)
}

// setPrevious is zend_exception_set_previous: add becomes the previous of
// the last exception of ex's chain, unless ex is already in add's chain.
// It takes over the caller's reference to add.
func (s *vmState) setPrevious(ex, add *object) {
	if ex == add || add == nil || ex == nil {
		if add != nil && ex == add {
			_ = s.releaseObject(add)
		}
		return
	}
	for {
		for anc := excGet(add, "previous"); anc.kind == valueObject; anc = excGet(anc.object, "previous") {
			if anc.object == ex {
				_ = s.releaseObject(add)
				return
			}
		}
		prev := excGet(ex, "previous")
		if prev.kind != valueObject {
			v := value{kind: valueObject, object: add}
			_ = s.excSet(ex, "previous", v)
			_ = s.releaseObject(add)
			return
		}
		ex = prev.object
		if ex == add {
			_ = s.releaseObject(add)
			return
		}
	}
}

// excTraceString is Throwable::getTraceAsString: the trace array as text.
func excTraceString(trace value) string {
	var b strings.Builder
	n := 0
	if trace.kind == valueArray {
		pos := 0
		for bk, ok := trace.array.next(&pos); ok; bk, ok = trace.array.next(&pos) {
			frame := bk.val.deref()
			if frame.kind != valueArray {
				continue
			}
			get := func(name string) value {
				if e, ok := frame.array.lookup(arrayKey{isString: true, text: name}); ok {
					return e.val.deref()
				}
				return value{}
			}
			b.WriteByte('#')
			b.WriteString(strconv.Itoa(n))
			b.WriteByte(' ')
			if file := get("file"); file.kind == valueString {
				b.WriteString(file.text)
				b.WriteByte('(')
				if line := get("line"); line.kind == valueInt {
					b.WriteString(strconv.FormatInt(line.integer, 10))
				} else {
					b.WriteByte('0')
				}
				b.WriteString("): ")
			} else {
				b.WriteString("[internal function]: ")
			}
			for _, part := range [3]string{"class", "type", "function"} {
				if v := get(part); v.kind == valueString {
					b.WriteString(v.text)
				}
			}
			b.WriteByte('(')
			if args := get("args"); args.kind == valueArray {
				apos, first := 0, true
				for a, ok := args.array.next(&apos); ok; a, ok = args.array.next(&apos) {
					if !first {
						b.WriteString(", ")
					}
					first = false
					if a.key.isString {
						b.WriteString(a.key.text)
						b.WriteString(": ")
					}
					b.WriteString(traceArg(a.val))
				}
			}
			b.WriteString(")\n")
			n++
		}
	}
	b.WriteByte('#')
	b.WriteString(strconv.Itoa(n))
	b.WriteString(" {main}")
	return b.String()
}

// excString is Exception::__toString: the exception and the chain of its
// previous ones, the oldest first, each as "Class: message in file:line"
// and its stack trace. The result is kept in the string property.
func (s *vmState) excString(o *object) (string, error) {
	str := ""
	seen := map[*object]bool{}
	for cur := o; cur != nil && !seen[cur]; {
		seen[cur] = true
		message, err := s.h.toString(excGet(cur, "message"))
		if err != nil {
			return "", err
		}
		file, err := s.h.toString(excGet(cur, "file"))
		if err != nil {
			return "", err
		}
		line := int64(0)
		if lv := excGet(cur, "line"); lv.kind == valueInt {
			line = lv.integer
		}
		if (cur.class == s.h.classes["typeerror"] || cur.class == s.h.classes["argumentcounterror"]) && strings.Contains(message, ", called in ") {
			message += " and defined"
		}
		head := cur.class.name
		if message != "" {
			head += ": " + message
		}
		head += " in " + file + ":" + strconv.FormatInt(line, 10) + "\nStack trace:\n" + excTraceString(excGet(cur, "trace"))
		if str != "" {
			head += "\n\nNext " + str
		}
		str = head
		prev := excGet(cur, "previous")
		if prev.kind != valueObject || !isThrowableClass(prev.object.class) {
			break
		}
		cur = prev.object
	}
	if err := s.excSet(o, "string", stringValue(str)); err != nil {
		return "", err
	}
	return str, nil
}

// excResult is the result of the native method kind without parameters on
// exception this: a borrowed value.
func (s *vmState) excResult(kind uint8, this *object) (value, error) {
	switch kind {
	case excGetMessage:
		return excGet(this, "message"), nil
	case excGetCode:
		return excGet(this, "code"), nil
	case excGetFile:
		return excGet(this, "file"), nil
	case excGetLine:
		return excGet(this, "line"), nil
	case excGetTrace:
		return excGet(this, "trace"), nil
	case excGetPrevious:
		return excGet(this, "previous"), nil
	case excGetSeverity:
		return excGet(this, "severity"), nil
	case excGetTraceAsString:
		return stringValue(excTraceString(excGet(this, "trace"))), nil
	case excToString:
		str, err := s.excString(this)
		return stringValue(str), err
	}
	return nullValue(), nil
}

// excTypeError is zend_argument_type_error for parameter index of a native
// method of owner.
func excTypeError(owner *class, n excNative, index int, args []value) error {
	p := n.params[index]
	want := p.typ
	if p.nullable {
		want = "?" + want
	}
	return &vmInternalFrameThrow{function: owner.name + "->" + n.name, args: append([]value(nil), args...),
		thrown: &Throwable{"TypeError", owner.name + "::" + n.name + "(): Argument #" + strconv.Itoa(index+1) + " ($" + p.name + ") must be of type " + want + ", " + valueName(args[index]) + " given"}}
}

// callException runs the native method of an exception class the pending
// call names (zend_exceptions.c). The result is in the destination
// register and the call is gone.
func (s *vmState) callException(rec *vmPendingCall, in *vmInstr) error {
	f := s.frames[len(s.frames)-1]
	n, known := excNatives[rec.native]
	if !known || rec.this == nil || !isThrowableClass(rec.this.class) {
		return s.internal("unknown native method %d", rec.native)
	}
	owner := excBase(rec.this.class)
	if m, ok := s.h.resolveMethod(rec.this.class, n.name, rec.this.class); ok && m != nil && m.owner != nil && m.native == rec.native {
		owner = m.owner
	} else if rec.native == excErrorConstruct {
		owner = throwableBuiltins["errorexception"]
	}
	args := s.args[rec.base:]
	for i := range args {
		if args[i].kind == valueReference {
			t := args[i].ref.target
			retain(t)
			old := args[i]
			args[i] = t
			if err := s.releaseValue(old); err != nil {
				return err
			}
		}
	}
	throw := func(class, message string) error {
		return &vmInternalFrameThrow{function: owner.name + "->" + n.name, args: append([]value(nil), s.args[rec.base:]...), thrown: &Throwable{class, message}}
	}
	if rec.named.kind == valueArray {
		pos := 0
		for b, ok := rec.named.array.next(&pos); ok; b, ok = rec.named.array.next(&pos) {
			index := -1
			for i, p := range n.params {
				if p.name == b.key.text {
					index = i
				}
			}
			if index < 0 {
				return &Throwable{"Error", "Unknown named parameter $" + b.key.text}
			}
			for len(s.args)-rec.base <= index {
				s.args = append(s.args, value{})
			}
			if s.args[rec.base+index].kind != valueUndefined {
				return &Throwable{"Error", "Named parameter $" + b.key.text + " overwrites previous argument"}
			}
			v := b.val.deref()
			if v.kind >= valueArray {
				retain(v)
			}
			s.args[rec.base+index] = v
		}
		named := rec.named
		rec.named = value{}
		if err := s.releaseValue(named); err != nil {
			return err
		}
		args = s.args[rec.base:]
	}
	if len(args) > len(n.params) {
		bound := "at most"
		if len(n.params) == 0 {
			bound = "exactly"
		}
		plural := "s"
		if len(n.params) == 1 {
			plural = ""
		}
		return throw("ArgumentCountError", owner.name+"::"+n.name+"() expects "+bound+" "+strconv.Itoa(len(n.params))+" argument"+plural+", "+strconv.Itoa(len(args))+" given")
	}
	// zend_parse_parameters: each argument given is converted to its
	// parameter's type; null is accepted for a nullable parameter, and for
	// a scalar one in weak mode with a deprecation.
	given := make([]bool, len(n.params))
	for i := range args {
		p := n.params[i]
		v := args[i]
		if v.kind == valueUndefined {
			continue // skipped by a named argument: the default
		}
		given[i] = true
		if v.kind == valueNull {
			if p.nullable {
				given[i] = false
				continue
			}
			if f.fn.strict {
				return excTypeError(owner, n, i, args)
			}
			s.h.warn(levelDeprecated, owner.name+"::"+n.name+"(): Passing null to parameter #"+strconv.Itoa(i+1)+" ($"+p.name+") of type "+p.typ+" is deprecated")
			if s.stopped() {
				return nil
			}
			if p.typ == "string" {
				args[i] = stringValue("")
			} else {
				args[i], _ = intValue(0, s.prog.width)
			}
			continue
		}
		if p.typ == "Throwable" {
			if v.kind != valueObject || !isThrowableClass(v.object.class) {
				return excTypeError(owner, n, i, args)
			}
			continue
		}
		out, ok, err := s.h.coerceTypedValue(nil, nil, p.typ, v, f.fn.strict)
		if err != nil {
			return err
		}
		if !ok {
			return excTypeError(owner, n, i, args)
		}
		args[i] = out
	}
	arg := func(i int) (value, bool) {
		if i < len(args) && given[i] {
			return args[i], true
		}
		return value{}, false
	}
	this := rec.this
	out := nullValue()
	switch rec.native {
	case excConstruct, excErrorConstruct:
		if v, ok := arg(0); ok {
			if err := s.excSet(this, "message", v); err != nil {
				return err
			}
		}
		if v, ok := arg(1); ok && v.integer != 0 {
			if err := s.excSet(this, "code", v); err != nil {
				return err
			}
		}
		prev := 2
		if rec.native == excErrorConstruct {
			prev = 5
			severity, _ := intValue(1, s.prog.width)
			if v, ok := arg(2); ok {
				severity = v
			}
			if err := s.excSet(this, "severity", severity); err != nil {
				return err
			}
			file, hasFile := arg(3)
			if hasFile {
				if err := s.excSet(this, "file", file); err != nil {
					return err
				}
			}
			if line, ok := arg(4); ok {
				if err := s.excSet(this, "line", line); err != nil {
					return err
				}
			} else if hasFile {
				zero, _ := intValue(0, s.prog.width)
				if err := s.excSet(this, "line", zero); err != nil {
					return err
				}
			}
		}
		if v, ok := arg(prev); ok {
			if err := s.excSet(this, "previous", v); err != nil {
				return err
			}
		}
	default:
		var err error
		if out, err = s.excResult(rec.native, this); err != nil {
			return err
		}
	}
	if s.stopped() {
		return nil
	}
	if out.kind >= valueArray {
		retain(out) // held across the cleanup below
	}
	err := s.dropArgs(len(s.args) - rec.base)
	if perr := s.popCall(); err == nil {
		err = perr
	}
	if serr := s.setReg(f.regs, in.a, out); err == nil {
		err = serr
	}
	if out.kind >= valueArray {
		if rerr := s.releaseValue(out); err == nil {
			err = rerr
		}
	}
	return err
}
