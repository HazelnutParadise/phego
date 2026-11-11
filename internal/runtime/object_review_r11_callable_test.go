package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R11CallableNames() []string {
	inputs := []string{"self::stat", "parent::stat", "static::childOnly", "self::inst", "self::missing", "\\self::stat", "SELF::stat", "W13R11P::inst", "W13R11C::inst", "self::childOnly", "parent::inst"}
	modes := []string{"P-arg", "C-arg", "O-arg", "P-sarg", "C-sarg", "P-ret", "C-ret", "O-ret", "P-sret", "C-sret", "P-ref", "C-ref", "O-ref", "P-sref", "C-sref", "P-union", "unscoped"}
	var names []string
	for i := range inputs {
		for _, shape := range []string{"string", "array"} {
			for _, mode := range modes {
				names = append(names, fmt.Sprintf("r11-callable-%d-%s-%s", i, shape, mode))
			}
		}
	}
	return names
}

func w13R11CallableCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(name, "-")
	i := 0
	if _, err := fmt.Sscanf(parts[2], "%d", &i); err != nil {
		t.Fatal(err)
	}
	shape, mode := parts[3], strings.Join(parts[4:], "-")
	inputs := []string{"self::stat", "parent::stat", "static::childOnly", "self::inst", "self::missing", "\\self::stat", "SELF::stat", "W13R11P::inst", "W13R11C::inst", "self::childOnly", "parent::inst"}
	if i < 0 || i >= len(inputs) {
		t.Fatalf("invalid callable case %q", name)
	}
	method := func(name string, static bool, parameterType, returnType string, byReference, returnsArgument bool) methodSpec {
		var parameters []methodParameter
		if parameterType != "" || returnsArgument {
			parameters = []methodParameter{{name: "c", typeName: parameterType, byReference: byReference}}
		}
		return methodSpec{name: name, static: static, parameters: parameters, returnType: returnType, call: func(_ *heap, _ value, args []value) (value, error) {
			if returnsArgument {
				return args[0], nil
			}
			return w13Str("body"), nil
		}}
	}
	p := w13Build(t, classSpec{name: "W13R11P", methods: []methodSpec{
		method("stat", true, "", "", false, false), method("inst", false, "", "", false, false),
		method("arg", false, "callable", "", false, false), method("sarg", true, "callable", "", false, false),
		method("ref", false, "callable", "", true, false), method("sref", true, "callable", "", true, false),
		method("ret", false, "", "callable", false, true), method("sret", true, "", "callable", false, true),
		method("unionString", false, "", "callable|string", false, true), method("unionArray", false, "", "callable|array", false, true),
	}})
	c := w13Build(t, classSpec{name: "W13R11C", parent: p, methods: []methodSpec{method("childOnly", true, "", "", false, false)}})
	o := w13Build(t, classSpec{name: "W13R11O", parent: p, methods: []methodSpec{
		method("arg", false, "callable", "", false, false), method("ref", false, "callable", "", true, false), method("ret", false, "", "callable", false, true),
	}})
	storage := w13Build(t, classSpec{name: "W13R11Storage", properties: []propertySpec{{name: "slot", typeName: "string|array"}}})
	for _, class := range []*class{p, c, o, storage} {
		w13Must(t, "register class", h.registerClass(class))
	}
	input := inputs[i]
	var v value
	var inputOwner value
	if shape == "string" {
		v = w13Str(input)
	} else {
		classPart, methodPart, ok := strings.Cut(input, "::")
		if !ok {
			t.Fatalf("invalid callable input %q", input)
		}
		v = h.newArrayValue()
		w13Must(t, "append callable class", h.appendValue(&v, w13Str(classPart)))
		w13Must(t, "append callable method", h.appendValue(&v, w13Str(methodPart)))
		inputOwner = v // appendValue has already given this location one owner.
	}
	class := p
	if strings.HasPrefix(mode, "C-") {
		class = c
	} else if strings.HasPrefix(mode, "O-") {
		class = o
	}
	obj := w13Instantiate(t, h, class)
	var owner value
	w13Must(t, "own receiver", h.assign(&owner, obj))
	var result any
	var storageOwner value
	var storageObject value
	var storageAlias value
	if mode == "unscoped" {
		result = []any{h.isObjectCallable(v, nil)}
	} else {
		method := "ret"
		arg := methodArgument{value: v}
		if strings.HasSuffix(mode, "-arg") {
			method = "arg"
		} else if strings.HasSuffix(mode, "-sarg") {
			method = "sarg"
		} else if strings.HasSuffix(mode, "-sret") {
			method = "sret"
		} else if strings.HasSuffix(mode, "-sref") {
			method = "sref"
		} else if strings.HasSuffix(mode, "-ref") {
			method = "ref"
		}
		byReferenceMode := strings.HasSuffix(mode, "-ref") || strings.HasSuffix(mode, "-sref")
		if byReferenceMode { // The by-reference source is a typed property slot.
			storageObject = w13Instantiate(t, h, storage)
			w13Must(t, "own typed storage", h.assign(&storageOwner, storageObject))
			must(t, h.writeObject(storageObject, "slot", v, storage, true))
			w13Must(t, "reference typed source", h.referenceObject(&storageAlias, storageObject, "slot", storage))
			arg = methodArgument{value: storageAlias, slot: &storageAlias}
		}
		if strings.HasSuffix(mode, "-union") {
			method = "unionString"
			if shape == "array" {
				method = "unionArray"
			}
		}
		line := 41
		if method == "arg" {
			line = 39
		} else if method == "ref" {
			line = 40
		} else if method == "sarg" {
			line = 36
		} else if method == "sref" {
			line = 38
		} else if method == "sret" {
			line = 37
		} else if strings.HasSuffix(mode, "-union") {
			line = 35
		}
		out, err := h.callMethodAt(obj, method, []methodArgument{arg}, nil, false, methodCallSite{file: "/w/review_r11_callable.php", line: line})
		if err != nil {
			result = w13Error(err)
		} else {
			result = []any{"ok"}
			w13Must(t, "discard callable return", h.discardMethodReturn(out))
		}
		if byReferenceMode {
			after, readErr := h.readObject(storageObject, "slot", storage)
			w13Must(t, "read typed source after call", readErr)
			same, compareErr := identical(after, v)
			w13Must(t, "compare typed source after call", compareErr)
			if !same {
				t.Fatalf("by-reference callable validation changed its typed source: %s", name)
			}
			w13Must(t, "release typed source alias", h.unset(&storageAlias))
			w13Must(t, "release typed storage", h.unset(&storageOwner))
		}
	}
	observed := w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
	w13Must(t, "release receiver", h.unset(&owner))
	if shape == "array" {
		w13Must(t, "release callable array", h.unset(&inputOwner))
	}
	if err := h.shutdown(&value{}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("retained objects/sources: %d/%d", h.liveObjects, len(h.typedSources))
	}
	return observed
}

func TestW13R11RelativeCallables(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			for _, name := range w13R11CallableNames() {
				t.Run(name, func(t *testing.T) {
					h, err := referenceHeap(width)
					w13Must(t, "newHeap", err)
					w13AssertOracleCase(t, width, name, w13R11CallableCase(t, h, name))
				})
			}
		})
	}
}
