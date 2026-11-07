package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R10LifeNames() []string {
	var names []string
	for _, mode := range []string{"direct", "globals"} {
		for _, op := range []string{"read", "write", "isset", "unset", "reference"} {
			for _, keep := range []string{"released", "kept"} {
				for _, failure := range []string{"none", "init", "dtor"} {
					names = append(names, fmt.Sprintf("r10-life-%s-%s-%s-%s", mode, op, keep, failure))
				}
			}
		}
	}
	for _, op := range []string{"read", "write", "isset", "unset", "reference"} {
		for _, target := range []string{"ephemeral", "external"} {
			for _, failure := range []string{"none", "init", "dtor"} {
				names = append(names, fmt.Sprintf("r10-life-proxy-%s-%s-%s", op, target, failure))
			}
		}
	}
	return names
}

func w13R10LifeCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(name, "-")
	var events []any
	throwDtor := parts[len(parts)-1] == "dtor"
	c := w13Build(t, classSpec{name: "W13R10LazyLife", properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(42)}}, destructor: func(*heap, value) error {
		events = append(events, "dtor")
		if throwDtor {
			return &Throwable{"Exception", "dtor"}
		}
		return nil
	}})
	mode, op := parts[2], parts[3]
	failure := parts[len(parts)-1]
	isProxy := mode == "proxy"
	isExternal := isProxy && parts[4] == "external"
	keep := !isProxy && parts[4] == "kept"
	var mainOwner, holder, backingOwner value
	var receiver, backing value
	if isProxy {
		if isExternal {
			backing = w13Instantiate(t, h, c)
			must(t, h.assign(&backingOwner, backing))
		}
		var proxy value
		var err error
		proxy, err = h.newLazyObject(c, true, func(h *heap, _ value) (value, error) {
			events = append(events, "init")
			cleanup := h.unset(&mainOwner)
			if cleanup != nil {
				return value{}, cleanup
			}
			if failure == "init" {
				return value{}, &Throwable{"Exception", "init"}
			}
			if isExternal {
				return backing, nil
			}
			return h.instantiate(c)
		})
		must(t, err)
		receiver = proxy
		must(t, h.assign(&mainOwner, proxy))
	} else {
		var err error
		receiver, err = h.newLazyObject(c, false, func(h *heap, _ value) (value, error) {
			events = append(events, "init")
			if cleanupErr := h.unset(&mainOwner); cleanupErr != nil {
				return value{}, cleanupErr
			}
			if failure == "init" {
				return value{}, &Throwable{"Exception", "init"}
			}
			return nullValue(), nil
		})
		must(t, err)
		must(t, h.assign(&mainOwner, receiver))
		if keep {
			must(t, h.assign(&holder, receiver))
		}
	}
	// Globals read and isset carry a VM operand owner across the operation.
	var operand value
	holdOperand := !isProxy && mode == "globals" && (op == "read" || op == "isset")
	if holdOperand {
		must(t, h.assign(&operand, receiver))
	}
	var opValue any
	var opErr error
	switch op {
	case "read":
		var v value
		v, opErr = h.readObject(receiver, "x", nil)
		if opErr == nil {
			opValue = w13Encode(v)
		}
	case "write":
		opErr = h.writeObject(receiver, "x", w13Int(7), nil, false)
	case "isset":
		var v bool
		v, opErr = h.issetObject(receiver, "x", nil)
		if opErr == nil {
			opValue = v
		}
	case "unset":
		opErr = h.unsetObject(receiver, "x", nil)
	case "reference":
		var alias value
		opErr = h.referenceObject(&alias, receiver, "x", nil)
		if opErr == nil {
			opValue = w13Encode(alias)
		}
		opErr = h.mergeObjectErrors(opErr, h.unset(&alias))
	}
	if holdOperand {
		opErr = h.mergeObjectErrors(opErr, h.unset(&operand))
	}
	var opResult any
	if opErr != nil {
		opResult = w13Error(opErr)
	} else {
		opResult = opValue
	}
	proxyAlive := mainOwner.kind != valueUndefined && mainOwner.kind != valueNull
	var result []any
	if isProxy {
		var peek any
		if isExternal {
			peek = w13ObserveValue(func() (value, error) { return h.readObject(backingOwner, "x", nil) })
		}
		beforeCleanup := append([]any{}, events...)
		cleanup := w13ObserveError(func() error { return h.unset(&backingOwner) })
		afterCleanup := append([]any{}, events...)
		result = []any{opResult, beforeCleanup, proxyAlive, peek, cleanup, afterCleanup}
	} else {
		beforeCleanup := append([]any{}, events...)
		cleanup := w13ObserveError(func() error { return h.unset(&holder) })
		afterCleanup := append([]any{}, events...)
		result = []any{opResult, beforeCleanup, proxyAlive, cleanup, afterCleanup}
	}
	throwDtor = false // The frozen fixture disables destructor failures before its final global cleanup.
	finalCleanup := h.mergeObjectErrors(h.unset(&mainOwner), h.unset(&holder))
	finalCleanup = h.mergeObjectErrors(finalCleanup, h.unset(&backingOwner))
	must(t, finalCleanup)
	must(t, h.shutdown(&value{}))
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("lifetime fixture retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13R10LazyReleasedReceiver(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R10LifeNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R10LifeCase(t, h, name))
			})
		}
	}
}
