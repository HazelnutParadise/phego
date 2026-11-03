package runtime

// REQ-018/021; T-035/036/041/042.

import "testing"

func w13R4WeakMapNames() []string {
	return []string{"r4-weakmap-empty", "r4-weakmap-sameentries", "r4-weakmap-clone", "r4-weakmap-differentkey", "r4-weakmap-differentvalue", "r4-weakmap-differentcount", "r4-weakmap-reversedinsertorder", "r4-weakmap-arrayvalues", "r4-weakmap-selfcycle", "r4-weakmap-mutualcycle", "r4-weakmap-callback-values", "r4-weakmap-sharedidentity", "r4-weakmap-ordinary-class"}
}

func w13R4WeakMapCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var maps [2]value
	var mapOwners, keyOwners, otherOwners []value
	own := func(v value) value { var x value; must(t, h.assign(&x, v)); return x }
	newMap := func() value {
		v, err := h.newWeakMapObject()
		must(t, err)
		x := own(v)
		mapOwners = append(mapOwners, x)
		return v
	}
	newKey := func() value {
		v, err := h.newObject(&class{name: "stdClass"})
		must(t, err)
		x := own(v)
		keyOwners = append(keyOwners, x)
		return v
	}
	put := func(m, k, v value) { must(t, h.writeWeakMap(m, k, v)) }
	k1, k2 := newKey(), newKey()
	maps[0], maps[1] = newMap(), newMap()
	calls := 0
	stringerClass := w13Build(t, classSpec{name: "W13R4Stringable", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) { calls++; return w13Str("value"), nil }}}})
	stringer := func() value { return w13Instantiate(t, h, stringerClass) }
	switch name {
	case "r4-weakmap-empty", "r4-weakmap-sharedidentity", "r4-weakmap-ordinary-class":
	case "r4-weakmap-sameentries", "r4-weakmap-clone":
		put(maps[0], k1, w13Int(1))
		put(maps[1], k1, w13Int(1))
		if name == "r4-weakmap-clone" {
			cl, err := h.cloneObject(maps[0], nil)
			must(t, err)
			must(t, h.assign(&mapOwners[1], cl))
			maps[1] = mapOwners[1]
		}
	case "r4-weakmap-differentkey":
		put(maps[0], k1, w13Int(1))
		put(maps[1], k2, w13Int(1))
	case "r4-weakmap-differentvalue":
		put(maps[0], k1, w13Int(1))
		put(maps[1], k1, w13Int(2))
	case "r4-weakmap-differentcount":
		put(maps[0], k1, w13Int(1))
		put(maps[1], k1, w13Int(1))
		put(maps[1], k2, w13Int(2))
	case "r4-weakmap-reversedinsertorder":
		put(maps[0], k1, w13Int(1))
		put(maps[0], k2, w13Int(2))
		put(maps[1], k2, w13Int(2))
		put(maps[1], k1, w13Int(1))
	case "r4-weakmap-arrayvalues":
		a0, a1 := h.newArrayValue(), h.newArrayValue()
		a0.array.insert(intKey(0), w13Int(1), h.width)
		a1.array.insert(intKey(0), w13Int(2), h.width)
		put(maps[0], k1, a0)
		put(maps[1], k1, a1)
	case "r4-weakmap-selfcycle":
		put(maps[0], k1, maps[0])
		put(maps[1], k1, maps[1])
	case "r4-weakmap-mutualcycle":
		put(maps[0], k1, maps[1])
		put(maps[1], k2, maps[0])
	case "r4-weakmap-callback-values":
		lazyClass := w13Build(t, classSpec{name: "W13R4WeakMapLazy", properties: []propertySpec{{name: "p", typeName: "int"}}})
		makeLazy := func() value {
			v, err := h.newLazyObject(lazyClass, false, func(*heap, value) (value, error) {
				calls++
				return value{}, &Throwable{"Exception", "lazy initializer called"}
			})
			must(t, err)
			return v
		}
		put(maps[0], k1, stringer())
		put(maps[1], k1, stringer())
		put(maps[0], k2, makeLazy())
		put(maps[1], k2, makeLazy())
	default:
		t.Fatalf("unknown case %q", name)
	}
	a, b := maps[0], maps[1]
	if name == "r4-weakmap-sharedidentity" {
		b = a
	}
	if name == "r4-weakmap-ordinary-class" {
		b = own(w13Instantiate(t, h, w13Build(t, classSpec{name: "W13R4Ordinary"})))
		otherOwners = append(otherOwners, b)
	}
	ab := w13ObserveValue(func() (value, error) { n, e := h.compare(a, b); return w13Int(int64(n)), e })
	ba := w13ObserveValue(func() (value, error) { n, e := h.compare(b, a); return w13Int(int64(n)), e })
	eq := w13ObserveValue(func() (value, error) { n, e := h.compare(a, b); return boolValue(n == 0), e })
	identicalResult := w13ObserveValue(func() (value, error) { yes, e := identical(a, b); return boolValue(yes), e })
	for i := range keyOwners {
		must(t, h.unset(&keyOwners[i]))
	}
	for i := range otherOwners {
		must(t, h.unset(&otherOwners[i]))
	}
	for i := range mapOwners {
		must(t, h.unset(&mapOwners[i]))
	}
	if len(h.live) != 0 {
		t.Fatal("WeakMap fixture retained request objects", len(h.live))
	}
	return w13Case{Result: []any{ab, ba, eq, identicalResult, calls}, Diagnostics: w13Diagnostics(h)}
}
