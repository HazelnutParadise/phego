package runtime

// REQ-018; T-035/036. Object identity and enum unordered comparison are
// distinct from ordinary property comparison. No Go pointer is exposed.
func (h *heap) compareObjectValues(a, b value, arrays map[*phpArray]bool) (comparison int, failure error) {
	if a.kind != valueObject || b.kind != valueObject {
		obj, other, sign := a, b, 1
		if obj.kind != valueObject {
			obj, other, sign = b, a, -1
		}
		switch other.kind {
		case valueBool:
			if other.boolean {
				return 0, nil
			}
			return sign, nil
		case valueNull, valueUndefined:
			return sign, nil
		case valueInt, valueFloat:
			name := "int"
			n := h.number(1)
			if other.kind == valueFloat {
				name = "float"
				n = floatValue(1)
			}
			h.warn(levelNotice, "Object of class "+obj.object.class.name+" could not be converted to "+name)
			c, err := h.compareIn(n, other, arrays)
			return sign * c, err
		case valueString:
			if _, ok := h.resolveMethod(obj.object.class, "__toString", nil); ok {
				text, err := h.toString(obj)
				if err != nil {
					return 0, err
				}
				if sign > 0 {
					return h.compareIn(str(text), other, arrays)
				}
				return h.compareIn(other, str(text), arrays)
			}
			return sign, nil
		default:
			return sign, nil
		}
	}
	x, y := a.object, b.object
	if x == y {
		return 0, nil
	}
	if x.class != y.class {
		return 1, nil
	}
	owners := make([]methodHold, 0, 2)
	h.holdMethodValue(a, &owners)
	h.holdMethodValue(b, &owners)
	defer func() {
		for _, owner := range owners {
			failure = h.mergeObjectErrors(failure, h.releaseMethodHold(owner))
		}
	}()
	var err error
	x, y, err = h.comparisonPropertiesPHP8512(x, y)
	if err != nil {
		return 0, err
	}
	if x.class != y.class {
		return 1, nil
	}
	if x.class == weakMapBuiltin {
		return 1, nil
	}
	if x.class.schema != nil && len(x.class.schema.enumCases) > 0 {
		return 1, nil
	}
	if h.comparingObjects[x] {
		return 1, &Throwable{"Error", ErrRecursiveDependency.Error()}
	}
	if h.comparingObjects == nil {
		h.comparingObjects = make(map[*object]bool)
	}
	h.comparingObjects[x] = true
	defer delete(h.comparingObjects, x)
	xn, yn := 0, 0
	x.props.each(func(_ string, _ *value) { xn++ })
	y.props.each(func(_ string, _ *value) { yn++ })
	if xn > yn {
		return 1, nil
	}
	if xn < yn {
		return -1, nil
	}
	result := 0
	var first error
	x.props.each(func(name string, v *value) {
		if result != 0 || first != nil {
			return
		}
		other, ok := y.props.lookup(name)
		if !ok || other.kind == valueUndefined {
			result = 1
			return
		}
		result, first = h.compareIn(*v, *other, arrays)
	})
	return result, first
}

// These two pinned PHP 8.5.12 profiles evaluate the property-table operands
// in different orders. This is evidence for these image profiles, not a rule
// for every target sharing their integer width.
func (h *heap) comparisonPropertiesPHP8512(left, right *object) (*object, *object, error) {
	first, second := &left, &right
	if h.width == 32 {
		first, second = second, first
	}
	var err error
	*first, err = h.initializeLazy(*first)
	if err != nil {
		return nil, nil, err
	}
	*second, err = h.initializeLazy(*second)
	return left, right, err
}
