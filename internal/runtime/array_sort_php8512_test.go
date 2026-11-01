package runtime

import (
	"fmt"
	"math"
)

// REQ-017; T-034: PHP clears lookup chains while sorting recursive arrays.
func (m *w12Machine) sortRecursive(op []any) (err error) {
	slot := m.slot(op[1])
	defer func() {
		if e := m.h.shutdown(slot); err == nil {
			err = e
		}
	}()
	var child value
	if err = m.h.appendValue(&child, w12Int(1)); err != nil {
		return err
	}
	if err = m.h.appendValue(&child, w12Int(2)); err != nil {
		return err
	}
	if err = m.h.appendValue(slot, child); err != nil {
		return err
	}
	if err = m.h.unset(&child); err != nil {
		return err
	}
	e, err := m.h.element(slot, w12Int(1))
	if err != nil {
		return err
	}
	if err = m.h.bind(e, slot); err != nil {
		return err
	}
	if err = m.h.sortArrayRegular(slot, op[2] == "asort"); err != nil {
		return err
	}
	for _, b := range slot.deref().array.buckets {
		v, ok, e := m.h.getElement(b.val, w12Int(0))
		if e != nil {
			return e
		}
		if !ok {
			return fmt.Errorf("recursive fixture lost key zero")
		}
		kind := "integer"
		if v.deref().kind == valueArray {
			kind = "array"
		} else if v.deref().kind != valueInt {
			return fmt.Errorf("unexpected recursive fixture kind %d", v.deref().kind)
		}
		key := w12Int(b.key.integer)
		if b.key.isString {
			key = str(b.key.text)
		}
		m.events = append(m.events, []any{"recursive-key", w12Encode(key), kind})
	}
	return nil
}

// REQ-017; T-033/T-034: shared fixture entry point. The fixture callbacks,
// not a second sorting implementation, define the comparison observations.
func (m *w12Machine) sortTrace(op []any) (err error) {
	function, ok := op[2].(string)
	if !ok {
		return fmt.Errorf("sort_trace function must be a string")
	}
	userCallback := function == "usort" || function == "uasort"
	if !userCallback && function != "sort" && function != "asort" {
		return fmt.Errorf("unsupported sort_trace function %q", function)
	}
	mode, ok := op[3].(string)
	if !ok {
		return fmt.Errorf("sort_trace mode must be a string")
	}
	throwAt, ok := op[4].(int64)
	if !ok || throwAt < 0 {
		return fmt.Errorf("sort_trace throwAt must be a nonnegative integer")
	}
	switch mode {
	case "one", "minus", "zero", "normal", "cycle", "bool", "boolfalse", "booltrue", "boolnested", "boolref", "boolwarntrue", "boolwarnfalse", "object", "objectwarn", "float", "floatwarn":
	default:
		return fmt.Errorf("unsupported sort_trace mode %q", mode)
	}

	calls := int64(0)
	if !userCallback {
		return m.h.sortArrayRegular(m.slot(op[1]), function == "asort")
	}
	var ret, alias value
	if mode == "boolref" {
		if err = m.h.assign(&ret, boolValue(false)); err != nil {
			return err
		}
		if err = m.h.bind(&alias, &ret); err != nil {
			return err
		}
		defer func() {
			for _, p := range []*value{&alias, &ret} {
				if e := m.h.unset(p); err == nil {
					err = e
				}
			}
		}()
	}
	callback := func(a, b value) (value, error) {
		m.events = append(m.events, []any{"compare", w12Encode(a), w12Encode(b)})
		calls++
		if throwAt != 0 && calls == throwAt {
			return value{}, &Throwable{"Exception", "comparison failed"}
		}
		switch mode {
		case "one":
			return w12Int(1), nil
		case "minus":
			return w12Int(-1), nil
		case "zero":
			return w12Int(0), nil
		case "normal":
			n, err := m.h.compare(a, b)
			return w12Int(int64(n)), err
		case "bool", "boolnested":
			if mode == "boolnested" && calls == 2 {
				var inner value
				if err := m.h.appendValue(&inner, w12Int(2)); err != nil {
					return value{}, err
				}
				if err := m.h.appendValue(&inner, w12Int(1)); err != nil {
					return value{}, err
				}
				err := m.h.sortArrayUser(&inner, false, func(x, y value) (value, error) { n, e := m.h.compare(x, y); return w12Int(int64(n)), e })
				if e := m.h.unset(&inner); err == nil {
					err = e
				}
				if err != nil {
					return value{}, err
				}
			}
			n, err := m.h.compare(a, b)
			return boolValue(n > 0), err
		case "boolfalse":
			return boolValue(false), nil
		case "booltrue":
			return boolValue(true), nil
		case "boolwarntrue":
			return boolValue(true), nil
		case "boolwarnfalse":
			return boolValue(false), nil
		case "object", "objectwarn":
			return m.h.newObject(&class{name: "stdClass"})
		case "float", "floatwarn":
			return floatValue(math.Inf(1)), nil
		case "boolref":
			n, e := m.h.compare(a, b)
			if e != nil {
				return value{}, e
			}
			if e = m.h.assign(&ret, boolValue(n > 0)); e != nil {
				return value{}, e
			}
			return alias, nil
		case "cycle":
			if a.kind != valueInt || b.kind != valueInt {
				return value{}, fmt.Errorf("cycle comparator requires integers")
			}
			if a.integer == b.integer {
				return w12Int(0), nil
			}
			if (a.integer%3+1)%3 == b.integer%3 {
				return w12Int(-1), nil
			}
			return w12Int(1), nil
		default:
			return value{}, fmt.Errorf("unsupported sort_trace mode %q", mode)
		}
	}
	if mode == "boolwarntrue" || mode == "boolwarnfalse" || mode == "objectwarn" || mode == "floatwarn" {
		return m.h.sortArrayUserWithDiagnostic(m.slot(op[1]), function == "uasort", callback, func(level int, message string) error {
			m.events = append(m.events, []any{"diagnostic", level, message})
			return &Throwable{"Exception", "comparison warning failed"}
		})
	}
	return m.h.sortArrayUser(m.slot(op[1]), function == "uasort", callback)
}
