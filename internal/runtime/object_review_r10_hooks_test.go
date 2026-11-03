package runtime

import (
	"fmt"
	"testing"
)

func TestW13R10HookHoldOwners(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, ownedReceiver := range []bool{false, true} {
			for _, rhsMode := range []string{"scalar", "object", "array", "same", "reference"} {
				for _, ownedRHS := range []bool{false, true} {
					for _, outMode := range []string{"null", "receiver", "rhs"} {
						t.Run(fmt.Sprintf("%d/receiver=%t/rhs=%s/rhsOwned=%t/out=%s", width, ownedReceiver, rhsMode, ownedRHS, outMode), func(t *testing.T) {
							h, err := newHeap(width)
							must(t, err)
							c := w13Build(t, classSpec{name: "W13R10HookOwners", properties: []propertySpec{{name: "x"}}})
							p := &c.schema.props[0]
							receiver := w13Instantiate(t, h, c)
							var receiverOwner value
							if ownedReceiver {
								must(t, h.assign(&receiverOwner, receiver))
							}
							var rhsOwner, cell, ref value
							var rhs value
							switch rhsMode {
							case "scalar":
								rhs = w13Int(7)
							case "same":
								rhs = receiver
							case "object":
								rhs = w13Instantiate(t, h, c)
								if ownedRHS {
									must(t, h.assign(&rhsOwner, rhs))
								}
							case "array":
								rhs = h.newArrayValue()
								if ownedRHS {
									must(t, h.assign(&rhsOwner, rhs))
								}
							case "reference":
								cell = w13Int(1)
								must(t, h.bind(&ref, &cell))
								rhs = ref
							}
							receiverCount, rhsCount := methodRefcount(receiver), methodRefcount(rhs)
							var wantOut value
							switch outMode {
							case "null":
								wantOut = nullValue()
							case "receiver":
								wantOut = receiver
							case "rhs":
								wantOut = rhs
							}
							out, err := h.runPropertyHook(receiver.object, p, func(_ *heap, this, raw value) (value, error) {
								wantReceiver := receiverCount + 1
								if rhsMode == "same" {
									wantReceiver++
								}
								if got := methodRefcount(this); got != wantReceiver {
									t.Errorf("callback receiver count=%d, want %d", got, wantReceiver)
								}
								if rhsMode == "object" || rhsMode == "array" || rhsMode == "reference" {
									if got, want := methodRefcount(raw), rhsCount+1; got != want {
										t.Errorf("callback raw RHS count=%d, want %d", got, want)
									}
								} else if rhsMode == "same" {
									if got, want := methodRefcount(raw), receiverCount+2; got != want {
										t.Errorf("callback same RHS count=%d, want %d", got, want)
									}
								} else if methodRefcount(raw) >= 0 {
									t.Errorf("scalar RHS unexpectedly has refcount %d", methodRefcount(raw))
								}
								return wantOut, nil
							}, rhs, false)
							must(t, err)
							if methodRefcount(receiver) != receiverCount || methodRefcount(rhs) != rhsCount {
								t.Fatalf("post-hook counts receiver=%d rhs=%d want=%d/%d", methodRefcount(receiver), methodRefcount(rhs), receiverCount, rhsCount)
							}
							if outMode == "receiver" && (out.kind != valueObject || out.object != receiver.object) {
								t.Fatalf("receiver result lost identity: %#v", out)
							}
							if outMode == "rhs" {
								if rhsMode == "reference" {
									if out.kind != valueInt || out.integer != 1 {
										t.Fatalf("reference result was not dereferenced: %#v", out)
									}
								} else if rhs.kind == valueObject {
									if out.kind != valueObject || out.object != rhs.object {
										t.Fatalf("object result lost identity: %#v", out)
									}
								} else if rhs.kind == valueArray && (out.kind != valueArray || out.array != rhs.array) {
									t.Fatalf("array result lost identity: %#v", out)
								} else if rhs.kind == valueInt && (out.kind != valueInt || out.integer != rhs.integer) {
									t.Fatalf("scalar result=%#v", out)
								}
							}
							if ownedReceiver {
								must(t, h.unset(&receiverOwner))
							} else {
								must(t, h.discard(receiver))
							}
							if rhsMode == "object" || rhsMode == "array" {
								if ownedRHS {
									must(t, h.unset(&rhsOwner))
								} else {
									must(t, h.discard(rhs))
								}
							}
							if rhsMode == "reference" {
								must(t, h.unset(&ref))
								must(t, h.unset(&cell))
							}
							if len(h.live) != 0 || len(h.typedSources) != 0 {
								t.Fatalf("retained values: live=%d typed=%d", len(h.live), len(h.typedSources))
							}
						})
					}
				}
			}
		}
	}
}

func TestW13R10HookCleanupOrder(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			h, err := newHeap(width)
			must(t, err)
			var events []string
			child := w13Build(t, classSpec{name: "W13R10HookCleanupChild", destructor: func(*heap, value) error {
				events = append(events, "rhs")
				return &Throwable{"Exception", "rhs cleanup"}
			}})
			receiverClass := w13Build(t, classSpec{name: "W13R10HookCleanupReceiver", properties: []propertySpec{{name: "x"}}, destructor: func(*heap, value) error {
				events = append(events, "receiver")
				return &Throwable{"Exception", "receiver cleanup"}
			}})
			receiver := w13Instantiate(t, h, receiverClass)
			rhs := w13Instantiate(t, h, child)
			var receiverOwner, rhsOwner value
			must(t, h.assign(&receiverOwner, receiver))
			must(t, h.assign(&rhsOwner, rhs))
			p := &receiverClass.schema.props[0]
			_, err = h.runPropertyHook(receiver.object, p, func(h *heap, _, _ value) (value, error) {
				return nullValue(), h.mergeObjectErrors(h.unset(&receiverOwner), h.unset(&rhsOwner))
			}, rhs, false)
			if err == nil {
				t.Fatal("hook cleanup should report destructor errors")
			}
			if fmt.Sprint(events) != "[rhs receiver]" {
				t.Fatalf("cleanup order=%v", events)
			}
			if len(h.live) != 0 || len(h.typedSources) != 0 {
				t.Fatalf("retained values: live=%d typed=%d", len(h.live), len(h.typedSources))
			}
		})
	}
}

func BenchmarkW13PropertyHookHolds(b *testing.B) {
	for _, mode := range []string{"get", "scalar-set", "object-set"} {
		b.Run(mode, func(b *testing.B) {
			h, err := newHeap(64)
			if err != nil {
				b.Fatal(err)
			}
			var c *class
			c, err = buildClass(classSpec{name: "W13R10HookBench", properties: []propertySpec{{name: "x", hasDefault: true, initial: int64(1), get: func(*heap, value, value) (value, error) { return w13Int(1), nil }, set: func(h *heap, this, v value) (value, error) {
				if err := h.writeObject(this, "x", v, nil, false); err != nil {
					return value{}, err
				}
				return nullValue(), nil
			}}}})
			if err != nil {
				b.Fatal(err)
			}
			receiver, err := h.instantiate(c)
			if err != nil {
				b.Fatal(err)
			}
			var receiverOwner, auxOwner value
			if err = h.assign(&receiverOwner, receiver); err != nil {
				b.Fatal(err)
			}
			aux, err := h.instantiate(c)
			if err != nil {
				b.Fatal(err)
			}
			if err = h.assign(&auxOwner, aux); err != nil {
				b.Fatal(err)
			}
			if _, err = h.readObject(receiver, "x", nil); err != nil {
				b.Fatal(err)
			}
			if mode == "scalar-set" {
				if err = h.writeObject(receiver, "x", w13Int(2), nil, false); err != nil {
					b.Fatal(err)
				}
			}
			if mode == "object-set" {
				if err = h.writeObject(receiver, "x", aux, nil, false); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				switch mode {
				case "get":
					if got, e := h.readObject(receiver, "x", nil); e != nil || got.kind != valueInt || got.integer != 1 {
						b.Fatalf("get=%#v err=%v", got, e)
					}
				case "scalar-set":
					if e := h.writeObject(receiver, "x", w13Int(2), nil, false); e != nil {
						b.Fatal(e)
					}
				case "object-set":
					if e := h.writeObject(receiver, "x", aux, nil, false); e != nil {
						b.Fatal(e)
					}
				}
			}
			b.StopTimer()
			if err = h.unset(&receiverOwner); err != nil {
				b.Fatal(err)
			}
			if err = h.unset(&auxOwner); err != nil {
				b.Fatal(err)
			}
			if len(h.live) != 0 || len(h.typedSources) != 0 {
				b.Fatalf("retained values: live=%d typed=%d", len(h.live), len(h.typedSources))
			}
		})
	}
}
