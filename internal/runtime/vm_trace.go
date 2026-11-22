package runtime

import (
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// traceArgLimit is exception_string_param_max_len's default.
const traceArgLimit = 15

// uncaught builds the completion of a throwable that left the top level:
// Throwable::__toString with the backtrace captured where it was raised,
// reported by zend_exception_error as "Uncaught %s\n  thrown" at the
// throwable's file and line.
func (s *vmState) uncaught(t *Throwable) completion {
	return s.uncaughtFrom(t, "", nil)
}

// uncaughtFrom optionally starts the backtrace with an internal function's
// frame, called from the current instruction.
func (s *vmState) uncaughtFrom(t *Throwable, internal string, args []value) completion {
	_, _, file, line := s.location()
	trace := s.backtrace(internal, args)
	head := t.Class
	if t.Message != "" {
		head += ": " + t.Message
	}
	// zend_verify_arg_error ends an argument TypeError's message with where
	// the function was called, and PHP then words the throw location as
	// where the function is defined.
	in := " in "
	if t.Class == "TypeError" && strings.Contains(t.Message, ", called in ") {
		in = " and defined in "
	}
	// The exception's backtrace holds the arguments of every frame: an
	// object only an argument refers to, directly or inside an array,
	// outlives the unwinding and is destroyed after the error is reported.
	hold := func(a value) {
		if a = a.deref(); a.kind == valueObject || a.kind == valueArray {
			retain(a)
			s.traceHeld = append(s.traceHeld, a)
		}
	}
	// The frame of an internal function (a method of Closure) comes first.
	for _, a := range args {
		hold(a)
	}
	for i := len(s.frames) - 1; i >= 1; i-- {
		f := s.frames[i]
		for _, a := range s.frameArgs(f) {
			hold(a)
		}
		// The frame of Closure::call or __invoke below it lists the
		// arguments again, after the object for call.
		if f.via == closureCall && f.this != nil {
			hold(value{kind: valueObject, object: f.this})
		}
		if f.via == closureCall || f.via == closureInvoke {
			for _, a := range s.frameArgs(f) {
				hold(a)
			}
		}
	}
	if len(s.traceHeld) > 0 && s.x.afterFatal == nil {
		s.x.afterFatal = s.releaseTraceHeld
	}
	text := "Uncaught " + head + in + vmFileName(file) + ":" + strconv.Itoa(line) + "\nStack trace:\n" + trace + "\n  thrown"
	s.lastFile, s.lastLine = file, line
	return completion{kind: endThrow, err: &vmUncaught{thrown: t, file: file, line: line, trace: trace, text: text}}
}

// traceName is the function of frame f as a backtrace shows it: Class->m
// for a method called on an object, Class::m for a static one.
func traceName(f *vmFrame) string {
	if f.fn.name == "{constant expression}" {
		return "[constant expression]"
	}
	if f.scope == nil {
		return f.fn.name
	}
	if f.this != nil {
		return f.scope.name + "->" + f.fn.name
	}
	return f.scope.name + "::" + f.fn.name
}

// backtrace is the stack as Throwable::getTraceAsString prints it,
// optionally starting with an internal function's frame called from the
// current instruction.
func (s *vmState) backtrace(internal string, args []value) string {
	_, _, file, line := s.location()
	var b strings.Builder
	n := 0
	frame := func(callerFile string, callerLine int, name string, args []value, named *phpArray) {
		b.WriteByte('#')
		b.WriteString(strconv.Itoa(n))
		b.WriteByte(' ')
		b.WriteString(vmFileName(callerFile))
		b.WriteByte('(')
		b.WriteString(strconv.Itoa(callerLine))
		b.WriteString("): ")
		b.WriteString(name)
		b.WriteByte('(')
		for i, a := range args {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(traceArg(a))
		}
		// The named arguments a variadic parameter collects follow, as
		// "name: value" (zend_fetch_debug_backtrace keeps their keys).
		if named != nil {
			sep := len(args) > 0
			for _, bk := range named.buckets {
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
		n++
	}
	if internal != "" {
		frame(file, line, internal, args, nil)
	}
	main := &s.prog.funcs[s.prog.entry]
	for i := len(s.frames) - 1; i >= 0; i-- {
		f := s.frames[i]
		if i == 0 {
			if f.fn != main {
				// Code the engine called with no PHP frame below it: a
				// destructor at request shutdown.
				b.WriteByte('#')
				b.WriteString(strconv.Itoa(n))
				b.WriteString(" [internal function]: ")
				b.WriteString(traceName(f))
				b.WriteByte('(')
				for j, a := range s.frameArgs(f) {
					if j > 0 {
						b.WriteString(", ")
					}
					b.WriteString(traceArg(a))
				}
				b.WriteString(")\n")
				n++
			}
			break
		}
		caller := s.frames[i-1]
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
		switch f.via {
		case closureCall:
			// Closure::call enters the closure itself: the closure's frame
			// has no PHP caller, and the method's frame follows it.
			b.WriteByte('#')
			b.WriteString(strconv.Itoa(n))
			b.WriteString(" [internal function]: ")
			b.WriteString(traceName(f))
			b.WriteByte('(')
			args := s.frameArgs(f)
			for j, a := range args {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(traceArg(a))
			}
			b.WriteString(")\n")
			n++
			if f.this != nil {
				args = append([]value{{kind: valueObject, object: f.this}}, args...)
			}
			frame(caller.fn.files[caller.pc], line, "Closure->call", args, named)
		case closureMagic:
			// A closure of a magic method calls __call itself; the frame
			// below is the method the closure stands for, with the
			// arguments __call received as an array.
			b.WriteByte('#')
			b.WriteString(strconv.Itoa(n))
			b.WriteString(" [internal function]: ")
			b.WriteString(traceName(f))
			b.WriteByte('(')
			args := s.frameArgs(f)
			for j, a := range args {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(traceArg(a))
			}
			b.WriteString(")\n")
			n++
			name, sep, list := "", "::", []value(nil)
			if f.this != nil {
				sep = "->"
			}
			if f.scope != nil {
				name = f.scope.name + sep
			}
			if f.closure != nil && f.closure.closure != nil {
				name += f.closure.closure.magic
			}
			if len(args) > 1 {
				if arr := args[1].deref(); arr.kind == valueArray {
					pos := 0
					for bk, ok := arr.array.next(&pos); ok; bk, ok = arr.array.next(&pos) {
						list = append(list, bk.val)
					}
				}
			}
			frame(caller.fn.files[caller.pc], line, name, list, nil)
		case closureInvoke:
			frame(caller.fn.files[caller.pc], line, traceName(f), s.frameArgs(f), named)
			frame(caller.fn.files[caller.pc], line, "Closure->__invoke", s.frameArgs(f), named)
		default:
			frame(caller.fn.files[caller.pc], line, traceName(f), s.frameArgs(f), named)
		}
	}
	b.WriteByte('#')
	b.WriteString(strconv.Itoa(n))
	b.WriteString(" {main}")
	return b.String()
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
