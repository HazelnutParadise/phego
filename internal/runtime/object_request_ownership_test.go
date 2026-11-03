package runtime

import (
	"context"
	"testing"
)

// REQ-018/019/021; T-036/038/042. A composite must not smuggle live
// mutable state across requests, including a reference to an immutable scalar.
func TestW13NestedRequestOwnership(t *testing.T) {
	for _, name := range []string{"array-object", "array-scalar", "reference-object", "reference-array", "reference-scalar"} {
		t.Run(name, func(t *testing.T) {
			first, second := testHeap(t), testHeap(t)
			victim, err := second.newObject(&class{name: "Victim"})
			must(t, err)
			var victimOwner value
			must(t, second.assign(&victimOwner, victim))
			foreign, err := first.newObject(&class{name: "Foreign"})
			must(t, err)
			var owner value
			must(t, first.assign(&owner, foreign))
			input := owner
			switch name {
			case "array-object", "array-scalar", "reference-array":
				array := first.newArrayValue()
				element := foreign
				if name == "array-scalar" {
					element = w13Int(7)
				}
				must(t, first.appendValue(&array, element))
				input = array
			case "reference-scalar":
				input = w13Int(7)
			}
			var destination value
			if name == "reference-object" || name == "reference-array" || name == "reference-scalar" {
				var alias value
				must(t, first.bind(&alias, &input))
				err = second.bind(&destination, &alias)
			} else {
				err = second.assign(&destination, input)
			}
			if err == nil {
				t.Fatal("foreign composite accepted", name)
			}
			if destination.kind != valueUndefined {
				t.Fatal("rejected write changed destination")
			}
			if foreign.object.freed || second.live[victim.object.handle] != victim.object || victim.object.freed {
				t.Fatal("ownership check damaged a request")
			}
		})
	}
}

func TestW13ForeignDestinationOwnership(t *testing.T) {
	for _, operation := range []string{"assign", "bind-source", "bind-destination", "unset", "release", "discard", "separate"} {
		t.Run(operation, func(t *testing.T) {
			first, second := testHeap(t), testHeap(t)
			foreign, err := first.newObject(&class{name: "Foreign"})
			must(t, err)
			var owner value
			must(t, first.assign(&owner, foreign))
			local := w13Int(1)
			var destination value
			switch operation {
			case "assign":
				err = second.assign(&owner, local)
			case "bind-source":
				err = second.bind(&destination, &owner)
			case "bind-destination":
				err = second.bind(&owner, &local)
			case "unset":
				err = second.unset(&owner)
			case "release":
				err = second.release(owner)
			case "discard":
				fresh, e := first.newObject(&class{name: "UnownedForeign"})
				must(t, e)
				err = second.discard(fresh)
				if fresh.object.freed {
					t.Fatal("discard freed a foreign temporary")
				}
			case "separate":
				array := first.newArrayValue()
				must(t, first.appendValue(&array, foreign))
				_, err = second.separate(&array)
			}
			if err == nil {
				t.Fatal("foreign request operation accepted", operation)
			}
			if owner.kind != valueObject || owner.object != foreign.object || foreign.object.freed || first.live[foreign.object.handle] != foreign.object {
				t.Fatal("rejection mutated the foreign request")
			}
		})
	}
}

func TestW13UnclaimedGraphOwnershipAtomic(t *testing.T) {
	first, second := testHeap(t), testHeap(t)
	foreign, err := first.newObject(&class{name: "Foreign"})
	must(t, err)
	var owner value
	must(t, first.assign(&owner, foreign))
	raw := newArray()
	retain(foreign)
	raw.insert(intKey(0), foreign, 64)
	var destination value
	if err := second.assign(&destination, value{kind: valueArray, array: raw}); err == nil {
		t.Fatal("unclaimed graph hid a foreign object")
	}
	if raw.owner != nil || destination.kind != valueUndefined || foreign.object.refcount != 2 {
		t.Fatal("failed claim partially changed graph ownership")
	}
	independent := newArray()
	independent.insert(intKey(0), w13Int(1), 64)
	destination = value{kind: valueArray, array: independent}
	if err := second.assign(&destination, foreign); err == nil {
		t.Fatal("foreign source accepted")
	}
	if independent.owner != nil || destination.array != independent {
		t.Fatal("failed source claimed the destination")
	}
	cycle := newArray()
	cycle.refcount = 1
	cycle.insert(intKey(0), value{kind: valueArray, array: cycle}, 64)
	var live value
	must(t, second.assign(&live, value{kind: valueArray, array: cycle}))
	if cycle.owner != second {
		t.Fatal("unclaimed cycle not adopted")
	}
	must(t, second.unset(&live))
	count, err := second.collectCycles()
	must(t, err)
	if count != 1 {
		t.Fatal("adopted cycle was not collectable", count)
	}
}

// A returned constructor-style array can become a mutable location without
// losing the owners held by its elements or retaining a discarded copy.
func TestW13TemporaryArrayMutationOwnership(t *testing.T) {
	h := testHeap(t)
	destroyed := 0
	object, err := h.newObject(&class{name: "ArrayMember", destructor: func(*heap, value) error { destroyed++; return nil }})
	must(t, err)
	var source, empty value
	must(t, h.assign(&source, h.newArrayValue()))
	must(t, h.appendValue(&source, object))
	must(t, h.assign(&empty, h.newArrayValue()))
	result, err := h.arith("+", source, empty)
	must(t, err)
	must(t, h.unset(&source))
	must(t, h.unset(&empty))
	must(t, h.appendValue(&result, w13Int(2)))
	must(t, h.unset(&result))
	if destroyed != 1 {
		t.Fatal("temporary array leaked its member owner", destroyed)
	}
	self := h.newArrayValue()
	must(t, h.appendValue(&self, self))
	child, ok, err := h.getElement(self, w13Int(0))
	must(t, err)
	if !ok || child.kind != valueArray || child.array == self.array || child.array.count() != 0 {
		t.Fatal("temporary self insertion made a cycle")
	}
	must(t, h.unset(&self))
}

func TestW13TemporaryReferenceLocationOwnership(t *testing.T) {
	t.Run("same-temporary-in-two-locations", func(t *testing.T) {
		h := testHeap(t)
		released := 0
		o, err := h.newObject(&class{name: "SharedTemporary", destructor: func(*heap, value) error { released++; return nil }})
		must(t, err)
		source, destination := o, o
		must(t, h.bind(&destination, &source))
		if released != 0 {
			t.Fatal("binding destroyed its live target")
		}
		must(t, h.unset(&destination))
		must(t, h.unset(&source))
		if released != 1 {
			t.Fatal("binding leaked its target", released)
		}
	})
	for _, foreach := range []bool{false, true} {
		for _, kind := range []string{"object", "array", "reference", "resource"} {
			t.Run(kind+map[bool]string{false: "/bind", true: "/foreach"}[foreach], func(t *testing.T) {
				h := testHeap(t)
				released := 0
				object, err := h.newObject(&class{name: "Member", destructor: func(*heap, value) error { released++; return nil }})
				must(t, err)
				input := object
				switch kind {
				case "array":
					var source value
					must(t, h.assign(&source, h.newArrayValue()))
					must(t, h.appendValue(&source, object))
					input = value{kind: valueArray, array: h.duplicate(source.array)}
					must(t, h.unset(&source))
				case "reference":
					retain(object)
					input = value{kind: valueReference, ref: &reference{owner: h, target: object}}
				case "resource":
					must(t, h.discard(object))
					released = 0
					input, err = h.newResource(context.Background(), "test", func(context.Context) error { released++; return nil })
					must(t, err)
				}
				original := input
				var alias value
				if foreach {
					err = h.bindForeachItem(&alias, &input)
				} else {
					err = h.bind(&alias, &input)
				}
				must(t, err)
				must(t, h.unset(&alias))
				if released != 0 {
					t.Fatal("first alias released the source", released)
				}
				must(t, h.unset(&input))
				if released != 1 || methodRefcount(original) != 0 {
					t.Fatal("temporary reference lost ownership", released, methodRefcount(original))
				}
			})
		}
	}
	for _, self := range []bool{false, true} {
		h := testHeap(t)
		released := 0
		o, err := h.newObject(&class{name: "Temporary", destructor: func(*heap, value) error { released++; return nil }})
		must(t, err)
		destination := o
		source := w13Int(1)
		if self {
			must(t, h.bind(&destination, &destination))
		} else {
			must(t, h.bind(&destination, &source))
			if released != 1 {
				t.Fatal("overwritten temporary was not released")
			}
			must(t, h.unset(&source))
		}
		must(t, h.unset(&destination))
		if released != 1 {
			t.Fatal("temporary self binding lost ownership", released)
		}
	}
}
