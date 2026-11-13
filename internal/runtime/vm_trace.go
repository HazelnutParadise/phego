package runtime

import (
	"math"
	"strconv"
	"strings"
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
	var b strings.Builder
	n := 0
	frame := func(callerFile string, callerLine int, name string, args []value) {
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
		b.WriteString(")\n")
		n++
	}
	if internal != "" {
		frame(file, line, internal, args)
	}
	for i := len(s.frames) - 1; i >= 1; i-- {
		f, caller := &s.frames[i], &s.frames[i-1]
		frame(caller.fn.files[caller.pc], int(caller.fn.lines[caller.pc]), f.fn.name, s.frameArgs(f))
	}
	b.WriteByte('#')
	b.WriteString(strconv.Itoa(n))
	b.WriteString(" {main}")
	trace := b.String()
	head := t.Class
	if t.Message != "" {
		head += ": " + t.Message
	}
	text := "Uncaught " + head + " in " + vmFileName(file) + ":" + strconv.Itoa(line) + "\nStack trace:\n" + trace + "\n  thrown"
	s.lastFile, s.lastLine = file, line
	return completion{kind: endThrow, err: &vmUncaught{thrown: t, file: file, line: line, trace: trace, text: text}}
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
