package runtime

import (
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/phpconst"
)

// W-17 stage 5: REQ-001, REQ-016, REQ-025, REQ-026. Constants, the @
// operator and the unhandled match case, as PHP 8.5.12 runs them.

// PHP's error level bits that @ never silences (E_FATAL_ERRORS of
// zend_errors.h): E_ERROR, E_PARSE, E_CORE_ERROR, E_COMPILE_ERROR,
// E_USER_ERROR and E_RECOVERABLE_ERROR.
const eFatalErrors = 1 | 4 | 16 | 64 | 256 | 4096

// setReporting changes the run's error_reporting; diagnostics outside it
// are neither displayed nor logged.
func (s *vmState) setReporting(v int) {
	s.reporting = v
	s.x.errors.excluded = ^v
}

// silence is ZEND_BEGIN_SILENCE after the old value was saved: a reporting
// mask with any non-fatal level keeps only its fatal levels.
func (s *vmState) silence() {
	if s.reporting&^eFatalErrors != 0 {
		s.setReporting(s.reporting & eFatalErrors)
	}
}

// unsilence is ZEND_END_SILENCE: the saved mask comes back when the current
// one has only fatal levels and the saved one had more. A nested @ saved a
// fatal-only mask, so its end restores nothing and the outer end does.
func (s *vmState) unsilence(saved value) error {
	saved = saved.deref()
	if saved.kind != valueInt {
		return s.internal("EndSilence on a register that BeginSilence did not write")
	}
	if s.reporting&^eFatalErrors == 0 && int(saved.integer)&^eFatalErrors != 0 {
		s.setReporting(int(saved.integer))
	}
	return nil
}

// lookupConstant finds a constant by its registered name: the run's own
// constants first, then the predefined ones. For an unqualified name used
// in a namespace (bytecode.FetchConstUnqualified) a miss retries with the
// unqualified part, as zend_get_constant_ex does. pre is the predefined
// entry, nil for a user constant.
func (s *vmState) lookupConstant(name string, unqualified bool) (v value, pre *phpconst.Constant, ok bool) {
	if v, ok := s.consts[name]; ok {
		return v, nil, true
	}
	if c, ok := phpconst.Lookup(s.prog.width, name); ok {
		return predefinedValue(c), &c, true
	}
	if unqualified {
		if short := phpconst.Short(name); short != name {
			return s.lookupConstant(short, false)
		}
	}
	return value{}, nil, false
}

// predefinedValue is the run-time value of a predefined constant. Resource
// and platform-dependent constants have none; the caller reports them.
func predefinedValue(c phpconst.Constant) value {
	switch c.Kind {
	case phpconst.Null:
		return nullValue()
	case phpconst.Bool:
		return boolValue(c.Bool)
	case phpconst.Int:
		return value{kind: valueInt, integer: c.Int}
	case phpconst.Float:
		return floatValue(c.Float)
	case phpconst.String:
		return stringValue(c.Str)
	}
	return value{}
}

// fetchConst is ZEND_FETCH_CONSTANT for a name PHP could not fold at
// compile time: an undefined constant throws Error with the name as
// written, a deprecated one raises its deprecation on every read.
func (s *vmState) fetchConst(regs []value, in *vmInstr) error {
	name := s.prog.consts[in.b].text
	v, pre, ok := s.lookupConstant(name, in.aux == bytecode.FetchConstUnqualified)
	if !ok {
		return &Throwable{"Error", "Undefined constant \"" + s.prog.consts[in.c].text + "\""}
	}
	if pre != nil {
		switch pre.Kind {
		case phpconst.Resource:
			return s.unsupported("the constant " + pre.Name + " (a resource)")
		case phpconst.Platform:
			return s.unsupported("the platform-dependent constant " + pre.Name)
		}
		if pre.Deprecated != "" {
			s.h.warn(levelDeprecated, pre.Deprecated)
			if s.stopped() {
				return nil
			}
		}
	}
	return s.setReg(regs, in.a, v)
}

// declareConst is ZEND_DECLARE_CONST: the value register is copied into the
// run's constants under the registered name. A name already defined, the
// special constants and __COMPILER_HALT_OFFSET__ only warn, as
// zend_register_constant does.
func (s *vmState) declareConst(regs []value, in *vmInstr) error {
	name := s.prog.consts[in.b].text
	v := regs[in.a].deref()
	if v.kind == valueUndefined {
		v = nullValue()
	}
	if _, _, exists := s.lookupConstant(name, false); exists || name == "__COMPILER_HALT_OFFSET__" || isSpecialConstant(name) {
		s.h.warn(levelWarning, "Constant "+name+" already defined, this will be an error in PHP 9")
		return nil
	}
	if s.consts == nil {
		s.consts = make(map[string]value)
	}
	if v.kind >= valueArray {
		retain(v)
	}
	s.consts[name] = v
	return nil
}

// isSpecialConstant is zend_get_special_const: true, false and null in any
// letter case.
func isSpecialConstant(name string) bool {
	return strings.EqualFold(name, "true") || strings.EqualFold(name, "false") || strings.EqualFold(name, "null")
}

// matchError is ZEND_MATCH_ERROR, zend_match_unhandled_error: the subject
// is described as a backtrace argument is (smart_str_append_scalar with
// exception_string_param_max_len), and a non-scalar by its type.
func (s *vmState) matchError(v value) error {
	v = v.deref()
	var desc string
	switch v.kind {
	case valueArray:
		desc = "of type array"
	case valueObject:
		desc = "of type object"
	case valueResource:
		desc = "of type resource"
	default:
		desc = traceArg(v)
	}
	return &Throwable{"UnhandledMatchError", "Unhandled match case " + desc}
}
