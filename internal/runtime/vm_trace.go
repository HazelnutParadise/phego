package runtime

import (
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// traceArgLimit is exception_string_param_max_len's default.
const traceArgLimit = 15

// vmTraceFrame is one frame of a backtrace, as zend_fetch_debug_backtrace
// records it. A frame with no PHP caller has internal set and prints as
// "[internal function]".
type vmTraceFrame struct {
	file           string
	line           int
	internal       bool
	class, typ, fn string
	args           []value
	named          *phpArray
}

// traceParts is the function of frame f as a backtrace records it: the
// class and "->" for a method called on an object, "::" for a static one.
func traceParts(f *vmFrame) (class, typ, fn string) {
	if f.fn.name == "{constant expression}" {
		return "", "", "[constant expression]"
	}
	if f.scope == nil {
		return "", "", f.fn.name
	}
	if f.this != nil {
		return f.scope.name, "->", f.fn.name
	}
	return f.scope.name, "::", f.fn.name
}

// traceName is the function of frame f as a backtrace shows it.
func traceName(f *vmFrame) string {
	class, typ, fn := traceParts(f)
	return class + typ + fn
}

// splitTraceName splits the name of an internal function's frame, such as
// Closure::bind or Exception->__construct, into its parts.
func splitTraceName(name string) (class, typ, fn string) {
	for _, sep := range [2]string{"::", "->"} {
		if i := strings.Index(name, sep); i > 0 {
			return name[:i], sep, name[i+2:]
		}
	}
	return "", "", name
}

// traceFrames is the stack as zend_fetch_debug_backtrace walks it,
// innermost first, optionally starting with an internal function's frame
// called from the current instruction. The argument values are borrowed.
func (s *vmState) traceFrames(internal string, args []value) []vmTraceFrame {
	_, _, file, line := s.location()
	var out []vmTraceFrame
	add := func(file string, line int, class, typ, fn string, args []value, named *phpArray) {
		out = append(out, vmTraceFrame{file: vmFileName(file), line: line, class: class, typ: typ, fn: fn, args: args, named: named})
	}
	addInternal := func(f *vmFrame, args []value) {
		class, typ, fn := traceParts(f)
		out = append(out, vmTraceFrame{internal: true, class: class, typ: typ, fn: fn, args: args})
	}
	if internal != "" {
		class, typ, fn := splitTraceName(internal)
		add(file, line, class, typ, fn, args, nil)
	}
	main := &s.prog.funcs[s.prog.entry]
	for i := len(s.frames) - 1; i >= s.traceFloor; i-- {
		f := s.frames[i]
		if f.leaving && i > s.traceFloor {
			// Its function has already returned. The method of Closure
			// that made the call (Closure::call, __invoke, the method a
			// closure of __call stands for) is still running.
			if f.via != 0 {
				lc := s.frames[i-1]
				lline := int(lc.fn.lines[lc.pc])
				largs := s.frameArgs(f)
				switch f.via {
				case closureCall:
					if f.this != nil {
						largs = append([]value{{kind: valueObject, object: f.this}}, largs...)
					}
					add(lc.fn.files[lc.pc], lline, "Closure", "->", "call", largs, nil)
				case closureInvoke:
					add(lc.fn.files[lc.pc], lline, "Closure", "->", "__invoke", largs, nil)
				case closureMagic:
					mclass, sep, magic, list := "", "", "", []value(nil)
					if f.scope != nil {
						mclass, sep = f.scope.name, "::"
						if f.this != nil {
							sep = "->"
						}
					}
					if f.closure != nil && f.closure.closure != nil {
						magic = f.closure.closure.magic
					}
					if len(largs) > 1 {
						if arr := largs[1].deref(); arr.kind == valueArray {
							pos := 0
							for bk, ok := arr.array.next(&pos); ok; bk, ok = arr.array.next(&pos) {
								list = append(list, bk.val)
							}
						}
					}
					add(lc.fn.files[lc.pc], lline, mclass, sep, magic, list, nil)
				}
			}
			continue
		}
		if i == s.traceFloor {
			if f.fn != main || i > 0 {
				// Code the engine called with no PHP frame below it: a
				// destructor at request shutdown.
				addInternal(f, s.frameArgs(f))
			}
			break
		}
		caller := s.frames[i-1]
		viaInternal := false
		for j := i - 1; caller.leaving && j > s.traceFloor; j-- {
			// Called while that frame's variables are released: the
			// call is made from the frame below it, or from Closure::call
			// or the closure of a magic method that entered it, which
			// are internal functions (__invoke is the closure's own
			// call and has a PHP caller).
			viaInternal = viaInternal || caller.via == closureCall || caller.via == closureMagic
			caller = s.frames[j-1]
		}
		if viaInternal {
			addInternal(f, s.frameArgs(f))
			continue
		}
		line := int(caller.fn.lines[caller.pc])
		var named *phpArray
		if c := f.call; c != nil {
			if c.named.kind == valueArray {
				named = c.named.array
			}
			// While the omitted parameters are being filled the call is
			// still at ZEND_CHECK_UNDEF_ARGS, which carries the line of the
			// last argument: the Checkpoint before CallPending.
			if c.holePC >= 0 && f.pc <= int(c.holePC) && caller.pc > 0 && caller.code[caller.pc-1].op == bytecode.OpCheckpoint {
				line = int(caller.fn.lines[caller.pc-1])
			}
		}
		class, typ, fn := traceParts(f)
		callerFile := caller.fn.files[caller.pc]
		switch f.via {
		case closureCall:
			// Closure::call enters the closure itself: the closure's frame
			// has no PHP caller, and the method's frame follows it.
			args := s.frameArgs(f)
			addInternal(f, args)
			if f.this != nil {
				args = append([]value{{kind: valueObject, object: f.this}}, args...)
			}
			add(callerFile, line, "Closure", "->", "call", args, named)
		case closureMagic:
			// A closure of a magic method calls __call itself; the frame
			// below is the method the closure stands for, with the
			// arguments __call received as an array.
			args := s.frameArgs(f)
			addInternal(f, args)
			mclass, sep, magic, list := "", "", "", []value(nil)
			if f.scope != nil {
				mclass, sep = f.scope.name, "::"
				if f.this != nil {
					sep = "->"
				}
			}
			if f.closure != nil && f.closure.closure != nil {
				magic = f.closure.closure.magic
			}
			if len(args) > 1 {
				if arr := args[1].deref(); arr.kind == valueArray {
					pos := 0
					for bk, ok := arr.array.next(&pos); ok; bk, ok = arr.array.next(&pos) {
						list = append(list, bk.val)
					}
				}
			}
			add(callerFile, line, mclass, sep, magic, list, nil)
		case closureInvoke:
			add(callerFile, line, class, typ, fn, s.frameArgs(f), named)
			add(callerFile, line, "Closure", "->", "__invoke", s.frameArgs(f), named)
		default:
			add(callerFile, line, class, typ, fn, s.frameArgs(f), named)
		}
	}
	return out
}

// backtrace is the stack as Throwable::getTraceAsString prints it,
// optionally starting with an internal function's frame called from the
// current instruction.
func (s *vmState) backtrace(internal string, args []value) string {
	var b strings.Builder
	frames := s.traceFrames(internal, args)
	for n, fr := range frames {
		b.WriteByte('#')
		b.WriteString(strconv.Itoa(n))
		if fr.internal {
			b.WriteString(" [internal function]: ")
		} else {
			b.WriteByte(' ')
			b.WriteString(fr.file)
			b.WriteByte('(')
			b.WriteString(strconv.Itoa(fr.line))
			b.WriteString("): ")
		}
		b.WriteString(fr.class)
		b.WriteString(fr.typ)
		b.WriteString(fr.fn)
		b.WriteByte('(')
		for i, a := range fr.args {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(traceArg(a))
		}
		// The named arguments a variadic parameter collects follow, as
		// "name: value" (zend_fetch_debug_backtrace keeps their keys).
		if fr.named != nil {
			sep := len(fr.args) > 0
			for _, bk := range fr.named.buckets {
				if bk == nil {
					continue
				}
				if sep {
					b.WriteString(", ")
				}
				sep = true
				b.WriteString(bk.key.text)
				b.WriteString(": ")
				b.WriteString(traceArg(bk.val))
			}
		}
		b.WriteString(")\n")
	}
	b.WriteByte('#')
	b.WriteString(strconv.Itoa(len(frames)))
	b.WriteString(" {main}")
	return b.String()
}

// traceArray is the backtrace as the array an exception object keeps
// (Throwable::getTrace): per frame file and line (when it has a PHP
// caller), function, class and type (for a method) and args. The array is
// owned by the caller and holds a reference to every argument, which is
// what keeps an object only an argument names alive while the exception
// is.
func (s *vmState) traceArray(frames []vmTraceFrame) value {
	h, width := s.h, s.prog.width
	key := func(name string) arrayKey { return arrayKey{isString: true, text: name} }
	out := h.newArrayValue()
	retain(out)
	for i, fr := range frames {
		row := h.newArrayValue()
		retain(row)
		if !fr.internal {
			row.array.insert(key("file"), stringValue(fr.file), width)
			line, _ := intValue(int64(fr.line), width)
			row.array.insert(key("line"), line, width)
		}
		row.array.insert(key("function"), stringValue(fr.fn), width)
		if fr.class != "" {
			row.array.insert(key("class"), stringValue(fr.class), width)
			row.array.insert(key("type"), stringValue(fr.typ), width)
		}
		args := h.newArrayValue()
		retain(args)
		for j, a := range fr.args {
			a = a.deref()
			if a.kind == valueUndefined {
				a = nullValue()
			}
			retain(a)
			args.array.insert(arrayKey{integer: int64(j)}, a, width)
		}
		if fr.named != nil {
			for _, bk := range fr.named.buckets {
				if bk == nil {
					continue
				}
				a := bk.val.deref()
				retain(a)
				args.array.insert(key(bk.key.text), a, width)
			}
		}
		row.array.insert(key("args"), args, width)
		out.array.insert(arrayKey{integer: int64(i)}, row, width)
	}
	return out
}

// uncaughtObject builds the completion of exception ex (owned) that left
// the top level, as zend_exception_error reports it: the text of its
// __toString, user-defined or not, as "Uncaught %s\n  thrown" at the
// exception's file and line. The run keeps the object, and with it the
// arguments its backtrace holds, until the error is reported.
func (s *vmState) uncaughtObject(ex *object) completion {
	exv := value{kind: valueObject, object: ex}
	s.traceHeld = append(s.traceHeld, exv)
	if s.x.afterFatal == nil {
		s.x.afterFatal = s.releaseTraceHeld
	}
	str := ""
	m, has := s.h.resolveMethod(ex.class, "__toString", ex.class)
	if has && m != nil && m.native == 0 && m.fn != 0 {
		// PHP reports the exception after the frames are gone: the method
		// has no PHP caller.
		floor := s.traceFloor
		s.traceFloor = len(s.frames)
		out, err := s.invoke(&s.prog.funcs[m.fn-1], exv, m.owner, ex.class, nil)
		s.traceFloor = floor
		switch {
		case err != nil:
			inner, ok, fail := s.thrownObject(err)
			if !ok {
				s.ended = false
				s.stop(fail)
				return s.end
			}
			// The exception __toString threw is the one reported.
			return s.uncaughtObject(inner)
		case out.kind != valueString:
			s.x.report("Warning", ex.class.name+"::__toString() must return a string", false)
			_ = s.releaseValue(out)
		default:
			_ = s.excSet(ex, "string", out)
		}
		if sv := excGet(ex, "string"); sv.kind == valueString {
			str = sv.text
		}
	} else {
		str, _ = s.excString(ex)
	}
	file, line := "", 0
	if fv := excGet(ex, "file"); fv.kind == valueString {
		file = fv.text
	}
	if lv := excGet(ex, "line"); lv.kind == valueInt {
		line = int(lv.integer)
	}
	message, _ := s.h.toString(excGet(ex, "message"))
	s.lastFile, s.lastLine = file, line
	text := "Uncaught " + str + "\n  thrown"
	return completion{kind: endThrow, err: &vmUncaught{thrown: &Throwable{ex.class.name, message}, file: file, line: line, trace: excTraceString(excGet(ex, "trace")), text: text}}
}

// frameArgs is debug_backtrace's view of a user frame's arguments: the
// current value of each passed parameter (NULL once unset), then the extra
// arguments. A parameter's value is in its variable once the prologue's
// Store has run, and until then still in its parameter register: that is
// the case while the default of a later parameter is evaluated, and for
// every parameter of a call that passed too few arguments, which stops
// before the prologue runs.
func (s *vmState) frameArgs(f *vmFrame) []value {
	passed := min(f.nargs, f.fn.params)
	out := make([]value, 0, f.nargs)
	for i := 0; i < passed; i++ {
		v := f.regs[i]
		if l := f.fn.paramLocal[i]; l >= 0 && !f.unreceived && f.pc > int(f.fn.paramPC[i]) {
			v = f.locals[l].deref()
		}
		out = append(out, v)
	}
	return append(out, f.extra...)
}

// traceArg is _build_trace_args: scalars by smart_str_append_scalar with
// strings escaped and cut to traceArgLimit bytes, arrays as "Array".
func traceArg(v value) string {
	switch v = v.deref(); v.kind {
	case valueUndefined, valueNull:
		return "NULL"
	case valueBool:
		if v.boolean {
			return "true"
		}
		return "false"
	case valueInt:
		return strconv.FormatInt(v.integer, 10)
	case valueFloat:
		out := php8512FormatFloat(v.floating, 14)
		if !math.IsInf(v.floating, 0) && !math.IsNaN(v.floating) && !strings.Contains(out, ".") {
			out += ".0"
		}
		return out
	case valueString:
		cut := v.text
		if len(cut) > traceArgLimit {
			cut = cut[:traceArgLimit]
		}
		out := "'" + escapeTraceString(cut)
		if len(v.text) > traceArgLimit {
			out += "..."
		}
		return out + "'"
	case valueArray:
		return "Array"
	case valueResource:
		return "Resource id #" + strconv.FormatUint(v.object.handle, 10)
	case valueObject:
		return "Object(" + v.object.class.name + ")"
	}
	return "NULL"
}

// escapeTraceString is smart_str_append_escaped.
func escapeTraceString(s string) string {
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 32 && c != '\\' && c <= 126 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('\\')
		switch c {
		case '\n':
			b.WriteByte('n')
		case '\r':
			b.WriteByte('r')
		case '\t':
			b.WriteByte('t')
		case '\f':
			b.WriteByte('f')
		case '\v':
			b.WriteByte('v')
		case '\\':
			b.WriteByte('\\')
		case 27:
			b.WriteByte('e')
		default:
			b.WriteByte('x')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	return b.String()
}

// releaseTraceHeld gives up the objects the reported error's backtrace
// held.
func (s *vmState) releaseTraceHeld() {
	held := s.traceHeld
	s.traceHeld = nil
	for _, v := range held {
		_ = s.h.release(v)
	}
}
