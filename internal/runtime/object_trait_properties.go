package runtime

import (
	"bytes"
	"math"
	"slices"
	"strings"
)

// REQ-018; T-035/036. Trait imports share a declaration only when their
// flags, invariant type and strict default agree. No request values are built.
func compatibleTraitProperty(a, b propertyInfo) bool {
	if a.visibility != b.visibility ||
		a.static != b.static || a.readonly != b.readonly || a.virtual != b.virtual ||
		a.get != nil || a.set != nil || b.get != nil || b.set != nil ||
		traitPropertyType(a.typeName, a.owner) != traitPropertyType(b.typeName, b.owner) {
		return false
	}
	// An omitted untyped default is null; an omitted typed default is UNDEF.
	aDefined := a.hasDefault || a.typeName == ""
	bDefined := b.hasDefault || b.typeName == ""
	if aDefined != bDefined {
		return false
	}
	if !aDefined {
		return true
	}
	if !a.hasDefault {
		a.initial = nil
	}
	if !b.hasDefault {
		b.initial = nil
	}
	remainingA, remainingB := 1<<20, 1<<20
	a32, aValid := traitDefaultDomain(a.initial, 1024, &remainingA)
	b32, bValid := traitDefaultDomain(b.initial, 1024, &remainingB)
	return aValid && bValid && a32 == b32 && identicalTraitDefault(a.initial, b.initial)
}

func traitPropertyType(typ string, owner *class) string {
	typ = strings.TrimSpace(typ)
	if strings.HasPrefix(typ, "?") {
		typ = typ[1:] + "|null"
	}
	var union []string
	for part := range splitMethodType(typ, '|') {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
			part = strings.TrimSpace(part[1 : len(part)-1])
		}
		if phpNameEqual(part, "iterable") {
			union = append(union, "array", "traversable")
			continue
		}
		var intersection []string
		for name := range splitMethodType(part, '&') {
			name = phpNameLower(strings.TrimPrefix(strings.TrimSpace(name), "\\"))
			if name == "self" && owner != nil {
				name = phpNameLower(owner.name)
			} else if name == "parent" && owner != nil && owner.schema != nil && owner.schema.parent != nil {
				name = phpNameLower(owner.schema.parent.name)
			}
			intersection = append(intersection, name)
		}
		slices.Sort(intersection)
		union = append(union, strings.Join(slices.Compact(intersection), "&"))
	}
	slices.Sort(union)
	return strings.Join(slices.Compact(union), "|")
}

// The schema builder has no platform. Preserve the input's importability on
// each width when discarding a duplicate declaration, including overwritten
// entries. Jointly 32-bit-valid snapshots have width-invariant equality.
func traitDefaultDomain(input any, depth int, remaining *int) (valid32, valid64 bool) {
	switch x := input.(type) {
	case nil, bool, float64, []byte:
		return true, true
	case int64:
		return x >= math.MinInt32 && x <= math.MaxInt32, true
	case []SnapshotEntry:
		if depth == 0 {
			return false, false
		}
		valid32 = true
		for _, entry := range x {
			if *remaining == 0 {
				return false, false
			}
			*remaining--
			switch key := entry.Key.(type) {
			case int64:
				valid32 = valid32 && key >= math.MinInt32 && key <= math.MaxInt32
			case string:
			default:
				return false, false
			}
			child32, child64 := traitDefaultDomain(entry.Value, depth-1, remaining)
			if !child64 {
				return false, false
			}
			valid32 = valid32 && child32
		}
		return valid32, true
	}
	return false, false
}

func identicalTraitDefault(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case int64:
		y, ok := b.(int64)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case []byte:
		y, ok := b.([]byte)
		return ok && bytes.Equal(x, y)
	case []SnapshotEntry:
		y, ok := b.([]SnapshotEntry)
		if !ok {
			return false
		}
		left, right := traitDefaultEntries(x), traitDefaultEntries(y)
		if len(left) != len(right) {
			return false
		}
		for i, entry := range left {
			if entry.key != right[i].key || !identicalTraitDefault(entry.value, right[i].value) {
				return false
			}
		}
		return true
	}
	return false
}

type traitDefaultEntry struct {
	key   arrayKey
	value any
}

func traitDefaultEntries(entries []SnapshotEntry) []traitDefaultEntry {
	var out []traitDefaultEntry
	indices := make(map[arrayKey]int, len(entries))
	for _, entry := range entries {
		var key arrayKey
		switch x := entry.Key.(type) {
		case int64:
			key = intKey(x)
		case string:
			key = stringKey(x)
			if integer, ok, _ := php8512IntegerStringKey(x, 64); ok {
				key = intKey(integer)
			}
		}
		if i, ok := indices[key]; ok {
			out[i].value = entry.Value
		} else {
			indices[key] = len(out)
			out = append(out, traitDefaultEntry{key, entry.Value})
		}
	}
	return out
}
