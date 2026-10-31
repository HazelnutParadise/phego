package runtime

import "fmt"

// SnapshotEntry is one ordered array element in a Go-owned snapshot. Key is
// int64 or string.
type SnapshotEntry struct {
	Key   any
	Value any
}

// exportSnapshot deep-copies a value into Go data that stays valid after the
// request heap closes: nil, bool, int64, float64, []byte, or []SnapshotEntry.
// Objects and undefined values are rejected so no live request handle escapes.
func exportSnapshot(v value) (any, error) {
	return snapshotValue(v, map[*phpArray]bool{})
}

func snapshotValue(v value, active map[*phpArray]bool) (any, error) {
	v = v.deref()
	switch v.kind {
	case valueNull:
		return nil, nil
	case valueBool:
		return v.boolean, nil
	case valueInt:
		return v.integer, nil
	case valueFloat:
		return v.floating, nil
	case valueString:
		return v.bytesCopy(), nil
	case valueArray:
		if active[v.array] {
			return nil, fmt.Errorf("snapshot cannot represent a recursive array")
		}
		active[v.array] = true
		defer delete(active, v.array)
		out := make([]SnapshotEntry, 0, v.array.count())
		pos := 0
		for b, ok := v.array.next(&pos); ok; b, ok = v.array.next(&pos) {
			item, err := snapshotValue(b.val, active)
			if err != nil {
				return nil, err
			}
			var key any = b.key.integer
			if b.key.isString {
				key = b.key.text
			}
			out = append(out, SnapshotEntry{Key: key, Value: item})
		}
		return out, nil
	case valueObject:
		return nil, fmt.Errorf("snapshot cannot export a live object handle")
	}
	return nil, fmt.Errorf("snapshot cannot export an undefined value")
}
