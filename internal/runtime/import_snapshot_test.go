package runtime

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestImportSnapshotPHP8512NormalizesKeysAndCopies(t *testing.T) {
	h, _ := newHeap(32)
	data := []byte{'a', 0, 'b'}
	input := []SnapshotEntry{{Key: "1", Value: data}, {Key: int64(1), Value: int64(7)}, {Key: "2147483648", Value: []SnapshotEntry{{Key: "nested", Value: data}}}}
	v, err := h.importSnapshotPHP8512(input, 8, 10)
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'z'
	got, err := exportSnapshot(v)
	if err != nil {
		t.Fatal(err)
	}
	want := []SnapshotEntry{{Key: int64(1), Value: int64(7)}, {Key: "2147483648", Value: []SnapshotEntry{{Key: "nested", Value: []byte{'a', 0, 'b'}}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if err := h.release(v); err != nil {
		t.Fatal(err)
	}
}

func TestImportSnapshotPHP8512RejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name            string
		input           any
		depth, elements int
		message         string
	}{
		{"wide integer", int64(math.MaxInt32) + 1, 4, 10, "32-bit"},
		{"wide key", []SnapshotEntry{{Key: int64(math.MaxInt32) + 1, Value: nil}}, 4, 10, "32-bit"},
		{"bad key", []SnapshotEntry{{Key: true, Value: nil}}, 4, 10, "key"},
		{"bad value", struct{}{}, 4, 10, "value"},
		{"depth", []SnapshotEntry{{Key: int64(0), Value: []SnapshotEntry{}}}, 1, 10, "depth"},
		{"elements", []SnapshotEntry{{Key: int64(0), Value: nil}}, 4, 0, "element"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newHeap(32)
			_, err := h.importSnapshotPHP8512(tc.input, tc.depth, tc.elements)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error %v, want %q", err, tc.message)
			}
		})
	}
	h, _ := newHeap(64)
	h.closed = true
	if _, err := h.importSnapshotPHP8512(nil, 4, 10); !errors.Is(err, ErrHeapClosed) {
		t.Fatalf("closed heap: %v", err)
	}
}

func TestImportSnapshotPHP8512WidthAndRecursiveLimit(t *testing.T) {
	input := []SnapshotEntry{{Key: "2147483648", Value: nil}}
	h, _ := newHeap(64)
	v, err := h.importSnapshotPHP8512(input, 4, 10)
	if err != nil {
		t.Fatal(err)
	}
	got, err := exportSnapshot(v)
	if err != nil {
		t.Fatal(err)
	}
	want := []SnapshotEntry{{Key: int64(2147483648), Value: nil}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if err := h.release(v); err != nil {
		t.Fatal(err)
	}
	cycle := make([]SnapshotEntry, 1)
	cycle[0] = SnapshotEntry{Key: int64(0), Value: cycle}
	if _, err := h.importSnapshotPHP8512(cycle, 8, 100); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("cycle error: %v", err)
	}
}
