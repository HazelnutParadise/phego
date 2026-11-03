package runtime

import (
	"errors"
	"fmt"
	"testing"
)

// REQ-018/020; T-035/036/039/040. A typed property reference that reaches the
// by-reference binder is checked as a reference, not converted as a fresh
// by-value argument. Ordinary untyped locations keep weak coercion/writeback.
func w13R6ReferenceNames() []string {
	return []string{
		"r6-param-direct-union-string", "r6-param-direct-int-float",
		"r6-param-direct-object-stringable", "r6-param-direct-mixed-stringable",
		"r6-param-direct-mixed-int", "r6-param-direct-union-int",
		"r6-param-direct-object-union", "r6-param-ordinary-int-string",
		"r6-param-untyped-property", "r6-param-ordinary-string-int",
		"r6-param-named-union-string", "r6-param-variadic-union-string",
		"r6-param-variadic-named", "r6-param-reflect-union-string",
		"r6-param-reflect-ordinary", "r6-param-callable-invoke",
		"r6-param-callable-array", "r6-param-callable-static",
		"r6-param-callable-reject", "r6-param-post-failure",
		"r6-param-variadic-second", "r6-param-variadic-named-second",
		"r6-param-variadic-fixedprefix", "r6-param-variadic-pos-named",
		"r6-param-literal", "r6-param-literal-pos-second", "r6-param-literal-named-second",
	}
}

func w13R6Attempt(fn func() (value, error)) any {
	v, err := fn()
	if err == nil {
		return w13Encode(v)
	}
	return w13R6Throwable(err)
}

func w13R6Throwable(err error) any {
	var php *Throwable
	if errors.As(err, &php) {
		return []any{php.Class, php.Message}
	}
	return []any{"Error", err.Error()}
}

type w13R6Fixture struct {
	t         *testing.T
	h         *heap
	source    value
	sink      value
	invokable *class
	events    []any
	owners    []*value
}

func w13R6NewFixture(t *testing.T, h *heap) *w13R6Fixture {
	t.Helper()
	f := &w13R6Fixture{t: t, h: h, events: []any{}}
	events := &f.events
	stringable := w13Build(t, classSpec{name: "W13R6Stringable", methods: []methodSpec{
		{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
			*events = append(*events, "convert")
			return w13Str("text"), nil
		}},
	}})
	invokable := w13Build(t, classSpec{name: "W13R6Invokable", methods: []methodSpec{
		{name: "__invoke", returnType: "string", call: func(*heap, value, []value) (value, error) {
			*events = append(*events, "invoke")
			return w13Str("invoked"), nil
		}},
		{name: "method", returnType: "string", call: func(*heap, value, []value) (value, error) {
			*events = append(*events, "method")
			return w13Str("method"), nil
		}},
		{name: "staticMethod", returnType: "string", static: true, call: func(*heap, value, []value) (value, error) {
			*events = append(*events, "static")
			return w13Str("static"), nil
		}},
	}})
	f.invokable = invokable
	source := w13Build(t, classSpec{name: "W13R6Source", properties: []propertySpec{
		{name: "unionInt", typeName: "int|string", hasDefault: true, initial: int64(1)},
		{name: "intProp", typeName: "int", hasDefault: true, initial: int64(1)},
		{name: "objUnion", typeName: "string|object"},
		{name: "mixed", typeName: "mixed"},
		{name: "obj", typeName: "object"},
		{name: "arr", typeName: "array"},
		{name: "staticStore", typeName: "array|string"},
	}})
	sink := w13Build(t, classSpec{name: "W13R6Sink", methods: []methodSpec{
		{name: "takeString", parameters: []methodParameter{{name: "v", typeName: "string", byReference: true}}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:string:"+typeName(a[0]))
			return w13Str("ok"), nil
		}},
		{name: "takeFloat", parameters: []methodParameter{{name: "v", typeName: "float", byReference: true}}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:float:"+typeName(a[0]))
			return w13Str("ok"), nil
		}},
		{name: "takeObject", parameters: []methodParameter{{name: "v", typeName: "object", byReference: true}}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:object:"+typeName(a[0]))
			return w13Str("ok"), nil
		}},
		{name: "takeInt", parameters: []methodParameter{{name: "v", typeName: "int", byReference: true}}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:int:"+typeName(a[0]))
			next := w13Int(a[0].deref().integer + 1)
			if err := h.assignPHP(&a[0], next, false); err != nil {
				return value{}, err
			}
			return w13Str("ok"), nil
		}},
		{name: "takeCallable", parameters: []methodParameter{{name: "v", typeName: "callable", byReference: true}}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:callable:"+typeName(a[0]))
			return w13Str("ok"), nil
		}},
		{name: "takeVariadic", parameters: []methodParameter{{name: "v", typeName: "string", byReference: true, variadic: true}}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:variadic:"+fmt.Sprint(a[0].deref().array.count()))
			return w13Str("ok"), nil
		}},
		{name: "takePrefix", parameters: []methodParameter{
			{name: "prefix", typeName: "string", byReference: true},
			{name: "v", typeName: "string", byReference: true, variadic: true},
		}, returnType: "string", call: func(_ *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:prefix:"+fmt.Sprint(a[1].deref().array.count()))
			return w13Str("ok"), nil
		}},
		{name: "takeNamed", parameters: []methodParameter{
			{name: "first", typeName: "string", byReference: true},
			{name: "second", typeName: "string", byReference: true},
		}, returnType: "string", call: func(h *heap, _ value, a []value) (value, error) {
			*events = append(*events, "body:named")
			return w13Str("ok"), nil
		}},
	}})
	f.source = f.own(w13Instantiate(t, h, source))
	f.sink = f.own(w13Instantiate(t, h, sink))
	object := w13Instantiate(t, h, stringable)
	for _, field := range []string{"objUnion", "mixed", "obj"} {
		must(t, h.writeObject(f.source, field, object, nil, false))
	}
	must(t, h.registerClass(invokable))
	return f
}

func (f *w13R6Fixture) own(v value) value {
	f.t.Helper()
	cell := new(value)
	must(f.t, f.h.assign(cell, v))
	f.owners = append(f.owners, cell)
	return *cell
}

func (f *w13R6Fixture) cell(v value) *value {
	f.t.Helper()
	cell := new(value)
	must(f.t, f.h.assign(cell, v))
	f.owners = append(f.owners, cell)
	return cell
}

func (f *w13R6Fixture) alias(obj value, field string) *value {
	f.t.Helper()
	cell := new(value)
	must(f.t, f.h.referenceObject(cell, obj, field, nil))
	f.owners = append(f.owners, cell)
	return cell
}

func (f *w13R6Fixture) read(obj value, field string) value {
	f.t.Helper()
	v, err := f.h.readObject(obj, field, nil)
	must(f.t, err)
	return f.own(v)
}

func (f *w13R6Fixture) readType(obj value, field string) any {
	f.t.Helper()
	return typeName(f.read(obj, field))
}

func (f *w13R6Fixture) eventsSnapshot() []any {
	return append([]any{}, f.events...)
}

func (f *w13R6Fixture) callAt(line int, method string, args ...methodArgument) any {
	f.t.Helper()
	return w13R6Attempt(func() (value, error) {
		return f.h.callMethodAt(f.sink, method, args, nil, false, methodCallSite{file: "/w/review_r6_references.php", line: line})
	})
}

func (f *w13R6Fixture) callInternal(method string, args ...methodArgument) any {
	f.t.Helper()
	return w13R6Attempt(func() (value, error) {
		return f.h.callMethodAt(f.sink, method, args, nil, false, methodCallSite{})
	})
}

func (f *w13R6Fixture) cleanup() {
	f.t.Helper()
	for i := len(f.owners) - 1; i >= 0; i-- {
		must(f.t, f.h.unset(f.owners[i]))
	}
	if len(f.h.live) != 0 || len(f.h.typedSources) != 0 {
		f.t.Fatalf("reference fixture retained objects=%d sources=%d", len(f.h.live), len(f.h.typedSources))
	}
}

func w13R6ReferenceCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	f := w13R6NewFixture(t, h)
	defer f.cleanup()
	var result any
	switch name {
	case "r6-param-direct-union-string":
		alias := f.alias(f.source, "unionInt")
		result = []any{
			f.callAt(41, "takeString", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "unionInt")),
		}
	case "r6-param-direct-int-float":
		alias := f.alias(f.source, "intProp")
		result = []any{
			f.callAt(46, "takeFloat", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "intProp")),
		}
	case "r6-param-direct-object-stringable":
		alias := f.alias(f.source, "objUnion")
		result = []any{
			f.callAt(51, "takeString", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "objUnion"),
		}
	case "r6-param-direct-mixed-stringable":
		alias := f.alias(f.source, "mixed")
		result = []any{
			f.callAt(56, "takeString", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "mixed"),
		}
	case "r6-param-direct-mixed-int":
		must(t, h.writeObject(f.source, "mixed", w13Int(1), nil, false))
		alias := f.alias(f.source, "mixed")
		result = []any{
			f.callAt(61, "takeString", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "mixed")),
		}
	case "r6-param-direct-union-int":
		alias := f.alias(f.source, "unionInt")
		result = []any{
			f.callAt(66, "takeInt", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "unionInt")),
		}
	case "r6-param-direct-object-union":
		alias := f.alias(f.source, "objUnion")
		result = []any{
			f.callAt(71, "takeObject", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "objUnion"),
		}
	case "r6-param-ordinary-int-string":
		cell := f.cell(w13Int(1))
		result = []any{
			f.callAt(76, "takeString", methodArgument{slot: cell}),
			f.eventsSnapshot(),
			w13Encode(*cell),
		}
	case "r6-param-untyped-property":
		dynamic := w13Build(t, classSpec{name: "stdClass", allowDynamic: true})
		object := f.own(w13Instantiate(t, h, dynamic))
		must(t, h.writeObject(object, "value", w13Int(1), nil, false))
		alias := f.alias(object, "value")
		result = []any{
			f.callAt(81, "takeString", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(object, "value")),
		}
	case "r6-param-ordinary-string-int":
		cell := f.cell(w13Str("7"))
		result = []any{
			f.callAt(86, "takeInt", methodArgument{slot: cell}),
			f.eventsSnapshot(),
			w13Encode(*cell),
		}
	case "r6-param-named-union-string":
		alias := f.alias(f.source, "unionInt")
		second := f.cell(w13Str("x"))
		result = []any{
			f.callAt(91, "takeNamed",
				methodArgument{name: "second", slot: second},
				methodArgument{name: "first", slot: alias},
			),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "unionInt")),
			w13Encode(*second),
		}
	case "r6-param-variadic-union-string":
		alias := f.alias(f.source, "unionInt")
		result = []any{
			f.callAt(96, "takeVariadic", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "unionInt")),
		}
	case "r6-param-variadic-named":
		alias := f.alias(f.source, "unionInt")
		result = []any{
			f.callAt(101, "takeVariadic",
				methodArgument{name: "b", value: w13Str("x")},
				methodArgument{name: "a", slot: alias},
			),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "unionInt")),
		}
	case "r6-param-variadic-second":
		good := f.cell(w13Str("ok"))
		alias := f.alias(f.source, "unionInt")
		result = []any{f.callAt(146, "takeVariadic", methodArgument{slot: good}, methodArgument{slot: alias}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-variadic-named-second":
		good := f.cell(w13Str("ok"))
		alias := f.alias(f.source, "unionInt")
		result = []any{f.callAt(151, "takeVariadic", methodArgument{name: "first", slot: good}, methodArgument{name: "second", slot: alias}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-variadic-fixedprefix":
		prefix := f.cell(w13Str("ok"))
		alias := f.alias(f.source, "unionInt")
		result = []any{f.callAt(157, "takePrefix", methodArgument{slot: prefix}, methodArgument{slot: alias}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-variadic-pos-named":
		first := f.cell(w13Str("ok"))
		alias := f.alias(f.source, "unionInt")
		result = []any{f.callAt(162, "takeVariadic", methodArgument{slot: first}, methodArgument{name: "second", slot: alias}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-literal":
		result = []any{f.callAt(167, "takeString", methodArgument{value: w13Str("literal")}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-literal-pos-second":
		good := f.cell(w13Str("ok"))
		result = []any{f.callAt(172, "takeVariadic", methodArgument{slot: good}, methodArgument{value: w13Str("literal")}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-literal-named-second":
		good := f.cell(w13Str("ok"))
		result = []any{f.callAt(177, "takeVariadic", methodArgument{name: "first", slot: good}, methodArgument{name: "second", value: w13Str("literal")}), f.eventsSnapshot(), w13Encode(f.read(f.source, "unionInt"))}
	case "r6-param-reflect-union-string":
		alias := f.alias(f.source, "unionInt")
		result = []any{
			f.callInternal("takeString", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			w13Encode(f.read(f.source, "unionInt")),
		}
	case "r6-param-reflect-ordinary":
		cell := f.cell(w13Int(1))
		result = []any{
			f.callInternal("takeString", methodArgument{slot: cell}),
			f.eventsSnapshot(),
			w13Encode(*cell),
		}
	case "r6-param-callable-invoke":
		object := w13Instantiate(t, h, f.invokable)
		must(t, h.writeObject(f.source, "mixed", object, nil, false))
		alias := f.alias(f.source, "mixed")
		result = []any{
			f.callAt(118, "takeCallable", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "mixed"),
		}
	case "r6-param-callable-array":
		object := w13Instantiate(t, h, f.invokable)
		array := h.newArrayValue()
		must(t, h.appendValue(&array, object))
		must(t, h.appendValue(&array, w13Str("method")))
		must(t, h.writeObject(f.source, "arr", array, nil, false))
		must(t, h.unset(&array))
		alias := f.alias(f.source, "arr")
		result = []any{
			f.callAt(123, "takeCallable", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "arr"),
		}
	case "r6-param-callable-static":
		must(t, h.writeObject(f.source, "staticStore", w13Str("W13R6Invokable::staticMethod"), nil, false))
		alias := f.alias(f.source, "staticStore")
		result = []any{
			f.callAt(128, "takeCallable", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "staticStore"),
		}
	case "r6-param-callable-reject":
		alias := f.alias(f.source, "obj")
		result = []any{
			f.callAt(133, "takeCallable", methodArgument{slot: alias}),
			f.eventsSnapshot(),
			f.readType(f.source, "obj"),
		}
	case "r6-param-post-failure":
		alias := f.alias(f.source, "intProp")
		failure := f.callAt(138, "takeString", methodArgument{slot: alias})
		bad := h.newArrayValue()
		write := w13R6Attempt(func() (value, error) {
			if err := h.writeObject(f.source, "intProp", bad, nil, false); err != nil {
				return value{}, err
			}
			return w13Str("write"), nil
		})
		must(t, h.discard(bad))
		recover := w13R6Attempt(func() (value, error) {
			if err := h.writeObject(f.source, "intProp", w13Int(2), nil, false); err != nil {
				return value{}, err
			}
			return w13Str("write"), nil
		})
		result = []any{failure, write, recover, w13Encode(f.read(f.source, "intProp"))}
	default:
		t.Fatalf("unknown W13 R6 reference case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
