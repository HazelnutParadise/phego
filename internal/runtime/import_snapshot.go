package runtime

import "fmt"

// importSnapshotPHP8512 converts an ordered Go snapshot into a request-owned
// value under the PHP 8.5.12 key rules. The returned value has one owner and
// must be released by the caller. Limits apply to the whole input tree.
func (h *heap) importSnapshotPHP8512(input any, maxDepth, maxElements int) (value, error) {
	if h.closed {
		return value{}, ErrHeapClosed
	}
	if maxDepth < 1 || maxDepth > 1024 {
		return value{}, fmt.Errorf("invalid snapshot depth limit %d: expected 1 through 1024", maxDepth)
	}
	if maxElements < 0 {
		return value{}, fmt.Errorf("invalid snapshot element limit %d: expected a nonnegative value", maxElements)
	}
	return h.importSnapshotValuePHP8512(input, maxDepth, &maxElements)
}

func (h *heap) importSnapshotValuePHP8512(input any, depth int, remaining *int) (value, error) {
	switch x := input.(type) {
	case nil:
		return nullValue(), nil
	case bool:
		return boolValue(x), nil
	case int64:
		return intValue(x, h.width)
	case float64:
		return floatValue(x), nil
	case []byte:
		return bytesValue(x), nil
	case []SnapshotEntry:
		if depth == 0 {
			return value{}, fmt.Errorf("snapshot exceeds the permitted array depth")
		}
		var out value
		if _, err := h.separate(&out); err != nil {
			return value{}, err
		}
		for i, entry := range x {
			if *remaining == 0 {
				_ = h.release(out)
				return value{}, fmt.Errorf("snapshot exceeds the permitted element count at entry %d", i)
			}
			*remaining--
			var key value
			var err error
			switch k := entry.Key.(type) {
			case int64:
				key, err = intValue(k, h.width)
			case string:
				key = bytesValue([]byte(k))
			default:
				err = fmt.Errorf("expected an integer or string key, got %T", entry.Key)
			}
			if err != nil {
				_ = h.release(out)
				return value{}, fmt.Errorf("snapshot entry %d key: %w", i, err)
			}
			child, err := h.importSnapshotValuePHP8512(entry.Value, depth-1, remaining)
			if err != nil {
				_ = h.release(out)
				return value{}, fmt.Errorf("snapshot entry %d value: %w", i, err)
			}
			err = h.setElement(&out, key, child)
			_ = h.release(child)
			if err != nil {
				_ = h.release(out)
				return value{}, fmt.Errorf("snapshot entry %d: %w", i, err)
			}
		}
		return out, nil
	default:
		return value{}, fmt.Errorf("unsupported snapshot value %T: expected null, bool, int64, float64, []byte, or ordered entries", input)
	}
}
