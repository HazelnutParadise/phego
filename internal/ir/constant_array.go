package ir

import (
	"iter"
	"math"
	"strconv"
)

// Values are immutable after evaluation. Union copies only the outer entries;
// nested arrays contain no references or request-local mutable storage.
type compileArray struct {
	entries   []compileEntry
	positions map[compileKey]int
	next      int64
	full      bool
	indexed   bool
}
type compileEntry struct {
	key   compileKey
	value compileConst
}
type compileKey struct {
	integer bool
	index   int64
	text    string
}

func (v compileConst) truth() (bool, bool) {
	if v.array {
		if v.elements == nil {
			return false, false
		}
		return len(v.elements.entries) != 0, true
	}
	if v.value == nil {
		return false, false
	}
	return constBool(v.value)
}

func (a *compileArray) put(key compileKey, value compileConst, width int) {
	if i, found := a.positions[key]; found {
		a.entries[i].value = value
		return
	}
	if key.integer && (!a.indexed || key.index >= a.next) && !a.full {
		a.indexed = true
		maxKey := int64(math.MaxInt64)
		if width == 32 {
			maxKey = math.MaxInt32
		}
		if key.index == maxKey {
			a.next = key.index
			a.full = true
		} else {
			a.next = key.index + 1
		}
	}
	a.positions[key] = len(a.entries)
	a.entries = append(a.entries, compileEntry{key, value})
}

func compileArrayKey(v compileConst, width int, budget *compileBudget) (compileKey, bool) {
	if v.array || v.value == nil {
		return compileKey{}, false
	}
	x := v.value
	switch x.Kind {
	case LitInt:
		return compileKey{integer: true, index: x.Int}, true
	case LitBool:
		var i int64
		if x.Bool {
			i = 1
		}
		return compileKey{integer: true, index: i}, true
	case LitFloat:
		i, ok := constInteger(x, width)
		return compileKey{integer: true, index: i}, ok
	case LitString:
		if !budget.spendWork(1 + len(x.Str)/64) {
			return compileKey{}, false
		}
		// PHP integer keys use canonical decimal spelling, not numeric-string rules.
		maxDigits := 20
		if width == 32 {
			maxDigits = 11
		}
		if len(x.Str) <= maxDigits {
			if i, err := strconv.ParseInt(x.Str, 10, width); err == nil && strconv.FormatInt(i, 10) == x.Str {
				return compileKey{integer: true, index: i}, true
			}
		}
		return compileKey{text: x.Str}, true
	}
	// In PHP 8.5 null and lossy floating keys produce runtime diagnostics.
	return compileKey{}, false
}

func (l *lowerer) compileConstArray(n *Node, depth int, budget *compileBudget) (compileConst, bool) {
	if !budget.spendWork(len(n.Kids)) {
		return compileConst{}, false
	}
	allConstant := true
	// All values must be constant before array-level key or unpack diagnostics.
	for _, item := range n.Kids {
		if item == nil {
			return compileConst{}, false
		}
		if item.Op == OpSpread && len(item.Kids) == 1 {
			_, ok := l.compileConst(item.Kids[0], depth+1, budget)
			allConstant = allConstant && ok
			continue
		}
		if item.Op != OpItem || len(item.Kids) != 2 || item.Flags&FlagRef != 0 {
			allConstant = false
			continue
		}
		_, ok := l.compileConst(item.Kids[1], depth+1, budget)
		allConstant = allConstant && ok
		if item.Kids[0] != nil {
			_, ok = l.compileConst(item.Kids[0], depth+1, budget)
			allConstant = allConstant && ok
		}
	}
	if !allConstant {
		return compileConst{}, false
	}
	entries := func(yield func(compileArrayElement) bool) {
		for _, child := range n.Kids {
			e := compileArrayElement{line: child.Line, spread: child.Op == OpSpread}
			if e.spread {
				e.value = budget.memo[child.Kids[0]].value
			} else {
				e.value = budget.memo[child.Kids[1]].value
				if key := child.Kids[0]; key != nil {
					e.key = budget.memo[key].value
					e.line = key.Line
				}
			}
			if !yield(e) {
				return
			}
		}
	}
	return l.assembleConstantArray(entries, len(n.Kids), l.unit.Profile.IntWidth, budget)
}

type compileArrayElement struct {
	key, value compileConst
	line       int
	spread     bool
}

// The two evaluation modes share insertion, key coercion and unpack rules.
// A lazy iterator avoids allocating another copy of all intermediate values.
func (l *lowerer) assembleConstantArray(entries iter.Seq[compileArrayElement], size, width int, budget *compileBudget) (compileConst, bool) {
	a := &compileArray{positions: make(map[compileKey]int, size)}
	for child := range entries {
		value := child.value
		if child.spread {
			if !value.array {
				if value.value == nil {
					return compileConst{}, false
				}
				kind := typeName(value.value)
				if value.value.Kind == LitBool {
					if value.value.Bool {
						kind = "true"
					} else {
						kind = "false"
					}
				}
				l.fail(child.line, "Only arrays and Traversables can be unpacked, %s given", kind)
			}
			if value.elements == nil {
				return compileConst{}, false
			}
			for _, item := range value.elements.entries {
				if !budget.spendWork(1 + len(item.key.text)/64) {
					return compileConst{}, false
				}
				key := item.key
				if key.integer {
					if a.full {
						return compileConst{}, false
					}
					key.index = a.next
				}
				a.put(key, item.value, width)
			}
			continue
		}
		key := compileKey{integer: true, index: a.next}
		if k := child.key; k.array || k.value != nil {
			if k.array {
				l.fail(child.line, "Illegal offset type")
			}
			var ok bool
			key, ok = compileArrayKey(k, width, budget)
			if !ok {
				return compileConst{}, false
			}
		} else if a.full {
			return compileConst{}, false
		}
		a.put(key, value, width)
	}
	return compileConst{array: true, elements: a}, true
}

func compileArrayUnion(a, b compileConst, budget *compileBudget) (compileConst, bool) {
	if a.elements == nil || b.elements == nil {
		return compileConst{}, false
	}
	if !budget.spendWork(len(a.elements.entries) + len(b.elements.entries)) {
		return compileConst{}, false
	}
	out := &compileArray{positions: make(map[compileKey]int), next: a.elements.next, full: a.elements.full}
	for _, src := range []*compileArray{a.elements, b.elements} {
		for _, e := range src.entries {
			if !budget.spendWork(len(e.key.text) / 64) {
				return compileConst{}, false
			}
			if _, exists := out.positions[e.key]; !exists {
				out.positions[e.key] = len(out.entries)
				out.entries = append(out.entries, e)
			}
		}
	}
	return compileConst{array: true, elements: out}, true
}

func compileArrayBinary(op string, a, b compileConst, width, depth int, budget *compileBudget) (compileConst, bool) {
	switch op {
	case "===", "!==", "==", "!=", "<>", "<", "<=", ">", ">=", "<=>":
	default:
		return compileConst{}, false
	}
	// Greater-than is reversed less-than: array comparisons are not a total order.
	if op == ">" {
		a, b = b, a
		op = "<"
	} else if op == ">=" {
		a, b = b, a
		op = "<="
	}
	c, ok := compileValueCompare(a, b, op == "===" || op == "!==", width, depth, budget)
	if !ok {
		return compileConst{}, false
	}
	return compileBinary(op, &Literal{Kind: LitInt, Int: int64(c)}, &Literal{Kind: LitInt}, width, budget)
}

func compileValueCompare(a, b compileConst, strict bool, width, depth int, budget *compileBudget) (int, bool) {
	if !budget.depth(depth) || !budget.spendWork(1) {
		return 0, false
	}
	if !a.array && !b.array {
		if a.value == nil || b.value == nil {
			return 0, false
		}
		if !budget.scan(a.value, b.value) {
			return 0, false
		}
		return constCompare(a.value, b.value, strict, width)
	}
	if a.array && b.array {
		x, y := a.elements, b.elements
		if x == nil || y == nil {
			return 0, false
		}
		if c := cmpInt(int64(len(x.entries)), int64(len(y.entries))); c != 0 {
			return c, true
		}
		for i, e := range x.entries {
			if !budget.spendWork(1 + len(e.key.text)/64) {
				return 0, false
			}
			j := i
			if strict {
				if e.key != y.entries[j].key {
					return 1, true
				}
			} else {
				var found bool
				j, found = y.positions[e.key]
				if !found {
					return 1, true
				}
			}
			c, ok := compileValueCompare(e.value, y.entries[j].value, strict, width, depth+1, budget)
			if !ok || c != 0 {
				return c, ok
			}
		}
		return 0, true
	}
	if strict {
		return 1, true
	}
	scalar := a
	if a.array {
		scalar = b
	}
	if scalar.value == nil {
		return 0, false
	}
	if scalar.value.Kind == LitBool || scalar.value.Kind == LitNull {
		x, ok := a.truth()
		if !ok {
			return 0, false
		}
		y, ok := b.truth()
		if !ok {
			return 0, false
		}
		if x == y {
			return 0, true
		}
		if !x {
			return -1, true
		}
		return 1, true
	}
	if a.array {
		return 1, true
	}
	return -1, true
}
