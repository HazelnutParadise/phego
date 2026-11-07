package runtime

import (
	"fmt"
	"strings"
	"testing"
)

// REQ-012/018; T-023/024/035/036. Effective magic handlers ignore visibility.
func w13R12MagicNames() []string {
	return []string{
		"r12-magic-public-missing",
		"r12-magic-public-secret",
		"r12-magic-public-guarded",
		"r12-magic-public-explicit",
		"r12-magic-public-callable-object",
		"r12-magic-public-callable-class",
		"r12-magic-protected-missing",
		"r12-magic-protected-secret",
		"r12-magic-protected-guarded",
		"r12-magic-protected-explicit",
		"r12-magic-protected-callable-object",
		"r12-magic-protected-callable-class",
		"r12-magic-private-missing",
		"r12-magic-private-secret",
		"r12-magic-private-guarded",
		"r12-magic-private-explicit",
		"r12-magic-private-callable-object",
		"r12-magic-private-callable-class",
		"r12-magic-child-forward",
		"r12-magic-child-explicit",
		"r12-magic-child-global",
		"r12-magic-child-callable",
	}
}

func w13R12MagicArray(t *testing.T, h *heap, prefix string, args []value) (value, error) {
	t.Helper()
	a := h.newArrayValue()
	if len(args) < 2 {
		return value{}, &Throwable{"Error", "magic callback expects name and arguments"}
	}
	if err := h.appendValue(&a, w13Str(prefix+args[0].deref().text)); err != nil {
		if a.array.refcount > 0 {
			_ = h.release(a)
		}
		return value{}, err
	}
	if err := h.appendValue(&a, args[1]); err != nil {
		if a.array.refcount > 0 {
			_ = h.release(a)
		}
		return value{}, err
	}
	// The returned array is an unowned constructor temporary. Transfer its
	// appendValue owner to that temporary without releasing its contents.
	a.array.refcount--
	return a, nil
}

func w13R12MagicCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var result any
	owners := []value{}
	own := func(v value) value {
		var owner value
		must(t, h.assign(&owner, v))
		owners = append(owners, owner)
		return owner
	}
	publicMagic := func(prefix string) w13Method {
		return func(h *heap, _ value, args []value) (value, error) {
			return w13R12MagicArray(t, h, prefix, args)
		}
	}
	public := []*class{}
	for _, vis := range []struct {
		name   string
		access access
	}{{"public", publicAccess}, {"protected", protectedAccess}, {"private", privateAccess}} {
		c := w13Build(t, classSpec{
			name: "W13R12Magic_" + vis.name,
			methods: []methodSpec{
				{name: "__call", visibility: vis.access, parameters: []methodParameter{{name: "name"}, {name: "args"}}, call: publicMagic("magic:")},
				{name: "secret", visibility: privateAccess, call: func(*heap, value, []value) (value, error) { return nullValue(), nil }},
				{name: "guarded", visibility: protectedAccess, call: func(*heap, value, []value) (value, error) { return nullValue(), nil }},
				{name: "__callStatic", static: true, visibility: vis.access, parameters: []methodParameter{{name: "name"}, {name: "args"}}, call: publicMagic("static:")},
			},
		})
		must(t, h.registerClass(c))
		public = append(public, c)
	}
	var parent *class
	parent = w13Build(t, classSpec{
		name: "W13R12MagicParent",
		methods: []methodSpec{
			{name: "__call", visibility: privateAccess, parameters: []methodParameter{{name: "name"}, {name: "args"}}, call: func(_ *heap, _ value, args []value) (value, error) {
				return w13Str("parent:" + args[0].deref().text), nil
			}},
			{name: "forward", parameters: []methodParameter{{name: "target"}}, call: func(h *heap, _ value, args []value) (value, error) {
				return h.callMethod(args[0], "missing", nil, parent, false)
			}},
			{name: "explicit", parameters: []methodParameter{{name: "target"}}, call: func(h *heap, _ value, args []value) (value, error) {
				return h.callMethod(args[0], "__call", []methodArgument{{value: w13Str("named")}, {value: h.newArrayValue()}}, parent, false)
			}},
		},
	})
	child := w13Build(t, classSpec{
		name:   "W13R12MagicChild",
		parent: parent,
		methods: []methodSpec{{name: "__call", visibility: protectedAccess, parameters: []methodParameter{{name: "name"}, {name: "args"}}, call: func(_ *heap, _ value, args []value) (value, error) {
			return w13Str("child:" + args[0].deref().text), nil
		}}},
	})
	must(t, h.registerClass(child))
	classByName := map[string]*class{"public": public[0], "protected": public[1], "private": public[2]}
	defer func() {
		for i := range owners {
			must(t, h.unset(&owners[i]))
		}
		var globals value
		must(t, h.shutdown(&globals))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatalf("magic fixture retained values after shutdown: live=%d typed=%d", h.liveObjects, len(h.typedSources))
		}
	}()

	call := func(receiver value, method string, args []methodArgument, scope *class) any {
		out, err := h.callMethod(receiver, method, args, scope, false)
		if err != nil {
			return w13Error(err)
		}
		encoded := w13Encode(out)
		must(t, h.discardMethodReturn(out))
		return encoded
	}
	if len(name) >= len("r12-magic-child-") && name[:len("r12-magic-child-")] == "r12-magic-child-" {
		receiver := own(w13Instantiate(t, h, child))
		switch name {
		case "r12-magic-child-forward":
			forwarder := own(w13Instantiate(t, h, child))
			result = call(forwarder, "forward", []methodArgument{{value: receiver}}, nil)
		case "r12-magic-child-explicit":
			forwarder := own(w13Instantiate(t, h, child))
			result = call(forwarder, "explicit", []methodArgument{{value: receiver}}, nil)
		case "r12-magic-child-global":
			result = call(receiver, "missing", nil, nil)
		case "r12-magic-child-callable":
			var callable value
			must(t, h.appendValue(&callable, receiver))
			must(t, h.appendValue(&callable, w13Str("missing")))
			owners = append(owners, callable)
			result = h.isObjectCallable(callable, nil)
		default:
			t.Fatal(name)
		}
	} else {
		parts := strings.Split(name, "-")
		visibility, mode := parts[2], strings.Join(parts[3:], "-")
		c := classByName[visibility]
		receiver := own(w13Instantiate(t, h, c))
		switch mode {
		case "callable-object":
			var callable value
			must(t, h.appendValue(&callable, receiver))
			must(t, h.appendValue(&callable, w13Str("missing")))
			owners = append(owners, callable)
			result = h.isObjectCallable(callable, nil)
		case "callable-class":
			var callable value
			must(t, h.appendValue(&callable, w13Str(c.name)))
			must(t, h.appendValue(&callable, w13Str("missing")))
			owners = append(owners, callable)
			result = h.isObjectCallable(callable, nil)
		case "explicit":
			result = call(receiver, "__call", []methodArgument{{value: w13Str("named")}, {value: h.newArrayValue()}}, nil)
		case "missing", "secret", "guarded":
			result = call(receiver, mode, []methodArgument{{value: w13Int(7)}, {name: "tail", value: w13Int(8)}}, nil)
		default:
			t.Fatal(name)
		}
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13R12MagicHandlers(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R12MagicNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R12MagicCase(t, h, name))
			})
		}
	}
}
