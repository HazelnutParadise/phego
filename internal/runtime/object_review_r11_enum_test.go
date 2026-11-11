package runtime

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
)

func w13R11EnumNames() []string {
	var names []string
	for _, kind := range []string{"unit", "int", "string"} {
		for _, rhs := range []string{"zero", "one", "two", "minus-one", "float-one", "float-two", "nan", "inf", "negative-inf", "empty-string", "numeric-string", "text", "empty-array", "array", "false", "true", "null", "object", "resource", "same", "other"} {
			names = append(names, "r11-enum-"+kind+"-"+rhs)
		}
	}
	return names
}

func w13R11EnumCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(name, "-")
	if len(parts) < 4 || parts[0] != "r11" || parts[1] != "enum" {
		t.Fatalf("invalid enum case name %q", name)
	}
	kindName := parts[2]
	rhsName := strings.Join(parts[3:], "-")
	className, enumType := "W13R11Unit", enumUnit
	var enumValues []any
	switch kindName {
	case "unit":
	case "int":
		className, enumType, enumValues = "W13R11Int", enumInt, []any{int64(1), int64(2)}
	case "string":
		className, enumType, enumValues = "W13R11String", enumString, []any{[]byte("a"), []byte("b")}
	default:
		t.Fatalf("unknown enum kind in %q", name)
	}
	enumClass := w13Build(t, classSpec{name: className, enumKind: enumType, enumCases: []string{"A", "B"}, enumValues: enumValues})
	left, err := h.enumCase(enumClass, "A")
	w13Must(t, "enumCase(A)", err)
	var leftOwner, rightOwner value
	w13Must(t, "own left enum", h.assign(&leftOwner, left))

	var right value
	switch rhsName {
	case "zero":
		right = w13Int(0)
	case "one":
		right = w13Int(1)
	case "two":
		right = w13Int(2)
	case "minus-one":
		right = w13Int(-1)
	case "float-one":
		right = floatValue(1)
	case "float-two":
		right = floatValue(2)
	case "nan":
		right = floatValue(math.NaN())
	case "inf":
		right = floatValue(math.Inf(1))
	case "negative-inf":
		right = floatValue(math.Inf(-1))
	case "empty-string":
		right = w13Str("")
	case "numeric-string":
		right = w13Str("1")
	case "text":
		right = w13Str("a")
	case "empty-array", "array":
		array := h.newArrayValue()
		if rhsName == "array" {
			w13Must(t, "append array element", h.appendValue(&array, w13Int(1)))
		}
		if rhsName == "array" {
			rightOwner = array // appendValue already acquired this location's owner.
		} else {
			w13Must(t, "own array", h.assign(&rightOwner, array))
		}
		right = array
	case "false":
		right = boolValue(false)
	case "true":
		right = boolValue(true)
	case "null":
		right = nullValue()
	case "object":
		ordinary := w13Build(t, classSpec{name: "W13R11Ordinary"})
		right, err = h.instantiate(ordinary)
		w13Must(t, "instantiate ordinary object", err)
		w13Must(t, "own ordinary object", h.assign(&rightOwner, right))
	case "resource":
		right, err = h.newResource(context.Background(), "stream", func(context.Context) error { return nil })
		w13Must(t, "new stream resource", err)
		w13Must(t, "own resource", h.assign(&rightOwner, right))
	case "same", "other":
		caseName := "A"
		if rhsName == "other" {
			caseName = "B"
		}
		right, err = h.enumCase(enumClass, caseName)
		w13Must(t, "enumCase("+caseName+")", err)
		w13Must(t, "own right enum", h.assign(&rightOwner, right))
	default:
		t.Fatalf("unknown enum RHS %q in %q", rhsName, name)
	}
	cache := h.enums[enumClass]
	cachedObjects := make(map[string]*object, len(cache))
	cachedRefcounts := make(map[string]int, len(cache))
	for caseName, cached := range cache {
		cachedObjects[caseName] = cached.object
		cachedRefcounts[caseName] = cached.object.refcount
	}
	leftCount := left.object.refcount
	var rightEnum *object
	if right.kind == valueObject && right.object.class == enumClass {
		rightEnum = right.object
	}
	rightCount := 0
	if rightEnum != nil {
		rightCount = rightEnum.refcount
	}

	// PHP evaluates loose equality, left spaceship, then right spaceship.
	result := []any{}
	for _, comparison := range [][2]value{{left, right}, {left, right}, {right, left}} {
		cmp, compareErr := h.compare(comparison[0], comparison[1])
		w13Must(t, "compare enum scalar", compareErr)
		if len(result) == 0 {
			result = append(result, cmp == 0)
		} else {
			result = append(result, cmp)
		}
	}
	currentCache := h.enums[enumClass]
	if len(currentCache) != len(cachedObjects) || left.object.refcount != leftCount || rightEnum != nil && rightEnum.refcount != rightCount {
		t.Fatalf("enum comparison changed cached cases or refcounts")
	}
	for caseName, cachedObject := range cachedObjects {
		current, ok := currentCache[caseName]
		if !ok || current.object != cachedObject || cachedObject.refcount != cachedRefcounts[caseName] {
			t.Fatalf("enum comparison changed cached case %q identity or refcount", caseName)
		}
	}
	observed := w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
	if rightOwner.kind != valueUndefined {
		w13Must(t, "unset right owner", h.unset(&rightOwner))
	}
	w13Must(t, "unset left owner", h.unset(&leftOwner))
	w13Must(t, "shutdown enum comparison heap", h.shutdown(&value{}))
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("enum comparison cleanup retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
	}
	return observed
}

func TestW13R11EnumScalarComparison(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R11EnumNames() {
			t.Run(name+"/"+strconv.Itoa(width), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R11EnumCase(t, h, name))
			})
		}
	}
}
