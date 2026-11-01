package phego

import (
	"bytes"
	"math"
	"testing"
)

func TestScalarValuesAreExplicit(t *testing.T) {
	if !Null().IsNull() {
		t.Fatal("null is not null")
	}
	if _, ok := Null().Int64(); ok {
		t.Fatal("null reported an integer")
	}
	if v, ok := Bool(true).Bool(); !ok || !v {
		t.Fatal("boolean changed")
	}
	if v, ok := Int64(math.MinInt64).Int64(); !ok || v != math.MinInt64 {
		t.Fatal("integer changed")
	}
	if v, ok := Float64(math.Inf(1)).Float64(); !ok || !math.IsInf(v, 1) {
		t.Fatal("float changed")
	}
}

func TestByteValuesOwnTheirInputAndOutput(t *testing.T) {
	input := []byte{'a', 0, 'b'}
	v := Bytes(input)
	input[0] = 'z'
	got, ok := v.Bytes()
	if !ok || !bytes.Equal(got, []byte{'a', 0, 'b'}) {
		t.Fatalf("got %v", got)
	}
	got[0] = 'x'
	again, _ := v.Bytes()
	if !bytes.Equal(again, []byte{'a', 0, 'b'}) {
		t.Fatalf("mutation escaped: %v", again)
	}
}

func TestOrderedArrayIsIsolated(t *testing.T) {
	input := []Entry{{Key: StringKey("first"), Value: Bytes([]byte("original"))}, {Key: IntKey(-1), Value: Int64(7)}}
	v := Array(input)
	input[0] = Entry{Key: IntKey(99), Value: Null()}
	entries, ok := v.Entries()
	if !ok || len(entries) != 2 {
		t.Fatalf("entries: %v", entries)
	}
	if key, ok := entries[0].Key.String(); !ok || key != "first" {
		t.Fatalf("first key: %v", entries[0].Key)
	}
	if key, ok := entries[1].Key.Int64(); !ok || key != -1 {
		t.Fatalf("second key: %v", entries[1].Key)
	}
	entries[1].Value = Null()
	again, _ := v.Entries()
	if number, ok := again[1].Value.Int64(); !ok || number != 7 {
		t.Fatal("entry mutation escaped")
	}
	if _, ok := Int64(1).Entries(); ok {
		t.Fatal("integer reported as array")
	}
}

func TestNestedArraysRemainIsolated(t *testing.T) {
	inner := Array([]Entry{{Key: IntKey(0), Value: Bytes([]byte("x"))}})
	outer := Array([]Entry{{Key: StringKey("inner"), Value: inner}})
	items, _ := outer.Entries()
	nested, ok := items[0].Value.Entries()
	if !ok || len(nested) != 1 {
		t.Fatalf("nested entries: %v", nested)
	}
	data, _ := nested[0].Value.Bytes()
	data[0] = 'y'
	nested[0].Value = Null()
	itemsAgain, _ := outer.Entries()
	nestedAgain, _ := itemsAgain[0].Value.Entries()
	got, _ := nestedAgain[0].Value.Bytes()
	if string(got) != "x" {
		t.Fatalf("nested mutation escaped: %q", got)
	}
}

func TestEmptyArrayAndEmptyStringAreNotNull(t *testing.T) {
	array := Array(nil)
	if array.IsNull() {
		t.Fatal("empty array became null")
	}
	if entries, ok := array.Entries(); !ok || len(entries) != 0 {
		t.Fatal("empty array not preserved")
	}
	text := Bytes(nil)
	if text.IsNull() {
		t.Fatal("empty string became null")
	}
	if data, ok := text.Bytes(); !ok || len(data) != 0 {
		t.Fatal("empty string not preserved")
	}
}

func TestValuesCanBeReadConcurrently(t *testing.T) {
	value := Array([]Entry{{Key: IntKey(0), Value: Bytes([]byte("safe"))}})
	results := make(chan bool, 32)
	for i := 0; i < cap(results); i++ {
		go func() {
			entries, ok := value.Entries()
			data, bytesOK := entries[0].Value.Bytes()
			valid := ok && bytesOK && string(data) == "safe"
			data[0] = 'x'
			entries[0].Value = Null()
			results <- valid
		}()
	}
	for i := 0; i < cap(results); i++ {
		if !<-results {
			t.Fatal("concurrent access changed a value")
		}
	}
}

// The copy completes before another goroutine modifies the caller-owned input.
func TestInputMutationAfterCopyIsIsolated(t *testing.T) {
	input := []byte("original")
	entries := []Entry{{Key: IntKey(0), Value: Int64(7)}}
	text := Bytes(input)
	array := Array(entries)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			input[0] = byte(i)
			entries[0].Value = Int64(int64(i))
		}
	}()
	for i := 0; i < 1000; i++ {
		got, ok := text.Bytes()
		if !ok || string(got) != "original" {
			t.Errorf("byte snapshot changed: %q", got)
			break
		}
		gotEntries, ok := array.Entries()
		if !ok || len(gotEntries) != 1 {
			t.Error("array snapshot changed")
			break
		}
		if value, ok := gotEntries[0].Value.Int64(); !ok || value != 7 {
			t.Errorf("array value changed: %d", value)
			break
		}
	}
	<-done
}
