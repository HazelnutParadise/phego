package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R13GraphNames() []string {
	labels := []string{"root", "left0", "right0", "left12", "right12", "parent", "child", "extra", "stringable", "unrelated", "missing", "case", "qualified"}
	var names []string
	for _, kind := range []string{"parent", "child"} {
		for _, label := range labels {
			names = append(names, "r13-graph-"+kind+"-"+label)
		}
	}
	names = append(names, "r13-deep-40-root", "r13-deep-40-missing", "r13-deep-80-root", "r13-deep-80-missing")
	return names
}

type w13R13Graph struct {
	root, unrelated, extra, parent, child *class
	targets                               map[string]*class
}

func w13R13GraphClasses(t *testing.T) w13R13Graph {
	t.Helper()
	g := w13R13Graph{}
	g.root = w13Build(t, classSpec{name: "W13R13Root", isInterface: true})
	g.unrelated = w13Build(t, classSpec{name: "W13R13Unrelated", isInterface: true})
	g.extra = w13Build(t, classSpec{name: "W13R13Extra", isInterface: true})
	lefts, rights := make([]*class, 13), make([]*class, 13)
	lefts[0] = w13Build(t, classSpec{name: "W13R13Left0", isInterface: true, interfaces: []*class{g.root}})
	rights[0] = w13Build(t, classSpec{name: "W13R13Right0", isInterface: true, interfaces: []*class{g.root}})
	for i := 1; i <= 12; i++ {
		lefts[i] = w13Build(t, classSpec{name: fmt.Sprintf("W13R13Left%d", i), isInterface: true, interfaces: []*class{lefts[i-1], rights[i-1]}})
		rights[i] = w13Build(t, classSpec{name: fmt.Sprintf("W13R13Right%d", i), isInterface: true, interfaces: []*class{lefts[i-1], rights[i-1]}})
	}
	g.parent = w13Build(t, classSpec{name: "W13R13Parent", interfaces: []*class{lefts[12], rights[12]}})
	g.child = w13Build(t, classSpec{name: "W13R13Child", parent: g.parent, interfaces: []*class{g.extra}, methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("child"), nil }}}})
	g.targets = map[string]*class{"root": g.root, "left0": lefts[0], "right0": rights[0], "left12": lefts[12], "right12": rights[12], "parent": g.parent, "child": g.child, "extra": g.extra, "stringable": stringableBuiltin, "unrelated": g.unrelated, "missing": nil, "case": g.root, "qualified": g.root}
	return g
}

func w13R13GraphCaseWithClasses(t *testing.T, h *heap, name string, g w13R13Graph) w13Case {
	t.Helper()
	if strings.HasPrefix(name, "r13-deep-") {
		return w13R13DeepGraphCase(t, h, name)
	}
	prefix, kindLabel, ok := strings.Cut(name, "r13-graph-")
	if prefix != "" || !ok {
		t.Fatalf("invalid graph case name %q", name)
	}
	kind, label, ok := strings.Cut(kindLabel, "-")
	if !ok || (kind != "parent" && kind != "child") {
		t.Fatalf("invalid graph case kind in %q", name)
	}
	receiverClass := g.parent
	if kind == "child" {
		receiverClass = g.child
	}
	spellings := map[string]string{"root": "W13R13Root", "left0": "W13R13Left0", "right0": "W13R13Right0", "left12": "W13R13Left12", "right12": "W13R13Right12", "parent": "W13R13Parent", "child": "W13R13Child", "extra": "W13R13Extra", "stringable": "Stringable", "unrelated": "W13R13Unrelated", "missing": "W13R13Missing", "case": "w13r13root", "qualified": `\W13R13Root`}
	spelling, ok := spellings[label]
	if !ok {
		t.Fatalf("unknown graph target %q in %q", label, name)
	}
	target, ok := g.targets[label]
	if !ok {
		t.Fatalf("unknown graph descriptor %q in %q", label, name)
	}
	obj := w13Instantiate(t, h, receiverClass)
	var owner value
	w13Must(t, "own graph receiver", h.assign(&owner, obj))
	nameMatch := classMatchesAnyName(obj.object.class, spelling)
	identityMatch := target != nil && classIs(obj.object.class, target)
	// The source adapter returns the name query, while the matching descriptor
	// query must agree without conflating name equality with pointer identity.
	if nameMatch != identityMatch {
		t.Fatalf("name and descriptor queries disagree for %s/%s: %t/%t", kind, label, nameMatch, identityMatch)
	}
	result := nameMatch
	diagnostics := w13Diagnostics(h)
	w13Must(t, "release graph receiver", h.unset(&owner))
	if err := h.shutdown(&value{}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatalf("retained objects/sources: %d/%d", len(h.live), len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: diagnostics}
}

func w13R13GraphCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	return w13R13GraphCaseWithClasses(t, h, name, w13R13GraphClasses(t))
}

func w13R13DeepGraphCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	depth := 40
	if strings.HasPrefix(name, "r13-deep-80-") {
		depth = 80
	}
	prefix := fmt.Sprintf("W13R13Deep%d", depth)
	root := w13Build(t, classSpec{name: prefix + "Root", isInterface: true})
	missing := w13Build(t, classSpec{name: prefix + "Missing", isInterface: true})
	left := w13Build(t, classSpec{name: prefix + "Left0", isInterface: true, interfaces: []*class{root}})
	right := w13Build(t, classSpec{name: prefix + "Right0", isInterface: true, interfaces: []*class{root}})
	for i := 1; i <= depth; i++ {
		previous := []*class{left, right}
		left = w13Build(t, classSpec{name: fmt.Sprintf("%sLeft%d", prefix, i), isInterface: true, interfaces: previous})
		right = w13Build(t, classSpec{name: fmt.Sprintf("%sRight%d", prefix, i), isInterface: true, interfaces: previous})
	}
	receiver := w13Build(t, classSpec{name: prefix + "Receiver", interfaces: []*class{left, right}})
	target := root
	if strings.HasSuffix(name, "-missing") {
		target = missing
	}
	obj := w13Instantiate(t, h, receiver)
	var owner value
	must(t, h.assign(&owner, obj))
	result := classMatchesAnyName(obj.object.class, target.name)
	if result != classIs(obj.object.class, target) {
		t.Fatal("deep name/identity queries disagree")
	}
	diagnostics := w13Diagnostics(h)
	must(t, h.unset(&owner))
	must(t, h.shutdown(&value{}))
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatal("deep graph retained request values")
	}
	return w13Case{Result: result, Diagnostics: diagnostics}
}

func TestW13R13InterfaceGraph(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			graph := w13R13GraphClasses(t)
			for _, name := range w13R13GraphNames() {
				t.Run(name, func(t *testing.T) {
					h, err := newHeap(width)
					w13Must(t, "newHeap", err)
					w13AssertOracleCase(t, width, name, w13R13GraphCaseWithClasses(t, h, name, graph))
				})
			}
		})
	}
}

func TestW13R13ClassTraversalControls(t *testing.T) {
	root := w13Build(t, classSpec{name: "W13R13ControlRoot", isInterface: true})
	missingSchema := &class{name: "W13R13Legacy"}
	branch := w13Build(t, classSpec{name: "W13R13Branch", isInterface: true, interfaces: []*class{root}})
	leaf := w13Build(t, classSpec{name: "W13R13Leaf", parent: missingSchema, interfaces: []*class{branch}})
	if !classIs(leaf, root) || !classMatchesAnyName(leaf, "W13R13ControlRoot") {
		t.Fatal("nil-schema parent suppressed pending interface branch")
	}
	legacyBranch := w13Build(t, classSpec{name: "W13R13LegacyBranch", isInterface: true, interfaces: []*class{missingSchema, root}})
	if !classIs(legacyBranch, root) || !classMatchesAnyName(legacyBranch, "W13R13ControlRoot") {
		t.Fatal("nil-schema interface suppressed its valid sibling")
	}
	if !classMatchesAnyName(leaf, `\W13R13ControlRoot`) || !classMatchesAnyName(leaf, "w13r13controlroot") {
		t.Fatal("leading slash or case-insensitive name lookup failed")
	}
	if classIs(nil, root) || classIs(leaf, nil) || classMatchesAnyName(nil, "W13R13ControlRoot") || classMatchesAnyName(leaf, "W13R13Absent") {
		t.Fatal("nil or missing lookup unexpectedly matched")
	}
	empty := &class{}
	if classMatchesAnyName(leaf) || classMatchesAnyName(empty) || !classMatchesAnyName(empty, "") {
		t.Fatal("empty target list confused with an empty legacy class name")
	}
	legacy := &class{name: "W13R13LegacyTarget"}
	if !classIs(legacy, legacy) || !classMatchesAnyName(legacy, "W13R13LegacyTarget") {
		t.Fatal("nil-schema identity/name fallback failed")
	}
	first := w13Build(t, classSpec{name: "W13R13Same", isInterface: true})
	second := w13Build(t, classSpec{name: "W13R13Same", isInterface: true})
	if !classMatchesAnyName(second, "W13R13Same") || classIs(second, first) {
		t.Fatal("name matching collapsed distinct descriptor identity")
	}
	parentOnly := w13Build(t, classSpec{name: "W13R13Ancestor", isInterface: true, interfaces: []*class{root}})
	child := w13Build(t, classSpec{name: "W13R13Descendant", parent: parentOnly})
	if !classIs(child, root) || !classMatchesAnyName(child, "W13R13ControlRoot") {
		t.Fatal("parent-only ancestor interface missed")
	}
	if !classMatchesAnyName(leaf, "W13R13Absent", "W13R13ControlRoot") {
		t.Fatal("OR names did not search later target")
	}
}

func TestW13R13GetterFastPathAllocations(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, sample := range []struct {
			name, typeName string
			raw            value
		}{{"untyped", "", w13Int(1)}, {"mixed", "mixed", w13Int(1)}, {"typed", "int", w13Int(1)}, {"coerced", "int", w13Str("1")}} {
			t.Run(fmt.Sprintf("%d/%s", width, sample.name), func(t *testing.T) {
				h, err := newHeap(width)
				w13Must(t, "newHeap", err)
				c := w13Build(t, classSpec{name: "W13R13Getter", properties: []propertySpec{{name: "x", typeName: sample.typeName, hasDefault: true, initial: int64(1), get: func(*heap, value, value) (value, error) { return sample.raw, nil }}}})
				obj := w13Instantiate(t, h, c)
				var owner value
				w13Must(t, "own getter receiver", h.assign(&owner, obj))
				var got value
				var readErr error
				allocs := testing.AllocsPerRun(100, func() { got, readErr = h.readObject(obj, "x", nil) })
				if readErr != nil || got.kind != valueInt || got.integer != 1 {
					t.Fatalf("getter=(%#v, %v)", got, readErr)
				}
				if allocs != 0 {
					t.Fatalf("getter allocations=%g, want 0", allocs)
				}
				w13Must(t, "release getter receiver", h.unset(&owner))
				if len(h.live) != 0 || len(h.typedSources) != 0 {
					t.Fatalf("retained objects/sources: %d/%d", len(h.live), len(h.typedSources))
				}
			})
		}
	}
}

var w13R13GraphBoolSink bool

func TestW13R13SuccessfulGraphAllocations(t *testing.T) {
	graph := w13R13GraphClasses(t)
	for _, named := range []bool{false, true} {
		t.Run(fmt.Sprint(named), func(t *testing.T) {
			var matched bool
			allocations := testing.AllocsPerRun(100, func() {
				if named {
					matched = classMatchesAnyName(graph.child, "W13R13Root")
				} else {
					matched = classIs(graph.child, graph.root)
				}
			})
			if !matched || allocations != 0 {
				t.Fatalf("successful query matched=%t allocations=%g, want true/0", matched, allocations)
			}
		})
	}
}

func BenchmarkW13InterfaceTraversal(b *testing.B) {
	for _, mode := range []string{"identity", "named"} {
		for _, shape := range []string{"plain-chain-32", "interface-chain-8", "interface-chain-32", "diamond-8", "diamond-16", "diamond-22"} {
			b.Run(mode+"/"+shape, func(b *testing.B) {
				var receiver, hit, sibling *class
				switch shape {
				case "plain-chain-32":
					hit = w13BenchClass("W13R13BenchPlain0", nil, nil)
					receiver = hit
					for i := 1; i < 32; i++ {
						receiver = w13BenchClass(fmt.Sprintf("W13R13BenchPlain%d", i), receiver, nil)
					}
				case "interface-chain-8", "interface-chain-32":
					n := 8
					if shape == "interface-chain-32" {
						n = 32
					}
					hit = w13BenchInterface("W13R13BenchIface0", nil)
					tail := hit
					for i := 1; i < n; i++ {
						tail = w13BenchInterface(fmt.Sprintf("W13R13BenchIface%d", i), []*class{tail})
					}
					receiver = w13BenchClass("W13R13BenchIfaceReceiver", nil, []*class{tail})
				default:
					n := 8
					if shape == "diamond-16" {
						n = 16
					}
					if shape == "diamond-22" {
						n = 22
					}
					hit = w13BenchInterface("W13R13BenchDiamondRoot", nil)
					left, right := w13BenchInterface("W13R13BenchDiamondLeft0", []*class{hit}), w13BenchInterface("W13R13BenchDiamondRight0", []*class{hit})
					for i := 1; i <= n; i++ {
						left, right = w13BenchInterface(fmt.Sprintf("W13R13BenchDiamondLeft%d", i), []*class{left, right}), w13BenchInterface(fmt.Sprintf("W13R13BenchDiamondRight%d", i), []*class{left, right})
					}
					receiver = w13BenchClass("W13R13BenchDiamondReceiver", nil, []*class{left, right})
					sibling = right
				}
				miss := w13BenchInterface("W13R13BenchUnrelated", nil)
				queries := []struct {
					name   string
					target *class
					want   bool
				}{{"hit-root", hit, true}, {"miss-unrelated", miss, false}}
				if sibling != nil {
					queries = append(queries, struct {
						name   string
						target *class
						want   bool
					}{"hit-sibling", sibling, true})
				}
				for _, queryCase := range queries {
					b.Run(queryCase.name, func(b *testing.B) {
						target := queryCase.target
						var query func() bool
						if mode == "identity" {
							query = func() bool { return classIs(receiver, target) }
						} else {
							query = func() bool { return classMatchesAnyName(receiver, target.name) }
						}
						if got := query(); got != queryCase.want {
							b.Fatalf("setup query=%t, want %t", got, queryCase.want)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							w13R13GraphBoolSink = query()
						}
						b.StopTimer()
					})
				}
			})
		}
	}
}

func w13BenchClass(name string, parent *class, interfaces []*class) *class {
	c, err := buildClass(classSpec{name: name, parent: parent, interfaces: interfaces})
	if err != nil {
		panic(err)
	}
	return c
}

func w13BenchInterface(name string, interfaces []*class) *class {
	c, err := buildClass(classSpec{name: name, interfaces: interfaces, isInterface: true})
	if err != nil {
		panic(err)
	}
	return c
}
