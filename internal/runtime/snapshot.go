package runtime

import (
	"context"
	"fmt"
)

// SnapshotEntry is one ordered array element in a Go-owned snapshot. Key is
// int64 or string.
type SnapshotEntry struct {
	Key   any
	Value any
}

// Bounds of an exported snapshot. They equal the bounds the root package
// gives importSnapshotPHP8512 for WithGlobal values, so a value that can be
// returned can also be passed back in.
const (
	// SnapshotMaxDepth is the deepest array nesting an export copies.
	SnapshotMaxDepth = 256
	// SnapshotMaxElements bounds the array elements an export copies, each
	// copy of a shared array counted again: PHP shares an array assigned
	// twice, so [$a, $a] nested twenty times is a few kilobytes in PHP and
	// more than a million elements as Go data.
	SnapshotMaxElements = 1 << 20
	// snapshotCheckEvery is how many elements an export copies between
	// checks of its context.
	snapshotCheckEvery = 4096
)

// exportSnapshot deep-copies a value into Go data that stays valid after the
// request heap closes: nil, bool, int64, float64, []byte, or []SnapshotEntry.
// Objects and undefined values are rejected so no live request handle escapes.
// It applies the snapshot bounds without a context.
func exportSnapshot(v value) (any, error) {
	return exportSnapshotBounded(context.Background(), v)
}

// exportSnapshotBounded is exportSnapshot that stops with a *LimitError
// (Resource "return-depth" or "return-elements") past the snapshot bounds,
// and with ctx's error when ctx ends, checked every snapshotCheckEvery
// elements.
func exportSnapshotBounded(ctx context.Context, v value) (any, error) {
	e := snapshotExport{ctx: ctx, active: map[*phpArray]bool{}}
	return e.value(v, 0)
}

type snapshotExport struct {
	ctx      context.Context
	active   map[*phpArray]bool
	elements int
}

func (e *snapshotExport) value(v value, depth int) (any, error) {
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
		if e.active[v.array] {
			return nil, fmt.Errorf("snapshot cannot represent a recursive array")
		}
		if depth == SnapshotMaxDepth {
			return nil, limitHit("return-depth", SnapshotMaxDepth, depth, 1)
		}
		e.active[v.array] = true
		defer delete(e.active, v.array)
		out := make([]SnapshotEntry, 0, min(v.array.count(), SnapshotMaxElements-e.elements))
		pos := 0
		for b, ok := v.array.next(&pos); ok; b, ok = v.array.next(&pos) {
			if e.elements == SnapshotMaxElements {
				return nil, limitHit("return-elements", SnapshotMaxElements, e.elements, 1)
			}
			if e.elements++; e.elements%snapshotCheckEvery == 0 {
				if err := e.ctx.Err(); err != nil {
					return nil, err
				}
			}
			item, err := e.value(b.val, depth+1)
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
