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
	direct := directObjectComparisonEligible(x, y)
	if !direct {
		var err error
		x, y, err = h.comparisonPropertiesPHP8512(x, y)
		if err != nil {
			return 0, err
		}
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
	if direct {
		return h.compareDeclaredObjectProperties(x, y, arrays, true)
	}
	xn, yn := objectHashPropertyCount(x), objectHashPropertyCount(y)
	if xn > yn {
		return 1, nil
	}
	if xn < yn {
		return -1, nil
	}
	return h.compareHashObjectProperties(x, y, arrays)
}

func directObjectComparisonEligible(x, y *object) bool {
	for _, o := range []*object{x, y} {
		if o == nil || o.class == nil || o.class.schema == nil ||
			o.props == nil || o.props.layout != o.class.schema.layout ||
			o.props.materialized || o.props.index != nil || o.props.head != nil || o.props.tail != nil {
			return false
		}
		if o.lazy != nil && (o.lazy.initialize != nil || o.lazy.proxy || o.lazy.target.kind == valueObject) {
			return false
		}
	}
	return true
}

func objectHashPropertyCount(o *object) int {
	declared := 0
	if o.props != nil {
		declared = len(o.props.declared)
		if o.props.index != nil {
			declared += len(o.props.index)
		}
	}
	return declared
}

func (h *heap) compareDeclaredObjectProperties(x, y *object, arrays map[*phpArray]bool, direct bool) (int, error) {
	if x.props.layout == nil {
		return 0, nil
	}
	for i := range x.props.layout.names {
		comparison, err := h.compareObjectPropertyValues(&x.props.declared[i], &y.props.declared[i], arrays, direct)
		if err != nil || comparison != 0 {
			return comparison, err
		}
	}
	return 0, nil
}

func (h *heap) compareHashObjectProperties(x, y *object, arrays map[*phpArray]bool) (int, error) {
	if comparison, err := h.compareDeclaredObjectProperties(x, y, arrays, false); err != nil || comparison != 0 {
		return comparison, err
	}

	result := 0
	var first error
	for block := x.props.head; block != nil; block = block.next {
		for i := 0; i < block.used; i++ {
			entry := &block.entries[i]
			if current, ok := x.props.index[entry.name]; !ok || current != &entry.value {
				continue
			}
			other, ok := y.props.lookup(entry.name)
			if !ok {
				result = 1
				break
			}
			result, first = h.compareObjectPropertyValues(&entry.value, other, arrays, false)
			if result != 0 || first != nil {
				break
			}
		}
		if result != 0 || first != nil {
			break
		}
	}
	return result, first
}

func (h *heap) compareObjectPropertyValues(left, right *value, arrays map[*phpArray]bool, direct bool) (int, error) {
	leftUndefined := left.deref().kind == valueUndefined
	rightUndefined := right.deref().kind == valueUndefined
	switch {
	case leftUndefined && rightUndefined:
		return 0, nil
	case leftUndefined || rightUndefined:
		if direct {
			return 1, nil
		}
		if leftUndefined {
			return -1, nil
		}
		return 1, nil
	default:
		return h.compareIn(*left, *right, arrays)
	}
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
	(*first).props.materialize()
	*second, err = h.initializeLazy(*second)
	if err == nil {
		(*second).props.materialize()
	}
	return left, right, err
}
