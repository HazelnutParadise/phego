package frontend

import (
	"errors"
	"strings"
	"testing"
)

func TestNestedPropertyHookParametersAreBounded(t *testing.T) {
	source := func(depth int) string {
		return "<?php class C { public $p { " + strings.Repeat("get($x {", depth) + "get;" + strings.Repeat("});", depth) + "} }"
	}
	if _, err := parse(t, source(100)); err != nil {
		t.Fatalf("valid nested property hooks rejected: %v", err)
	}
	_, err := parse(t, source(MaxNesting+10))
	var syntax *SyntaxError
	if !errors.As(err, &syntax) || !strings.Contains(syntax.Message, "nesting") {
		t.Fatalf("deep property hooks must return a located nesting error: %v", err)
	}
	if syntax.Line < 1 {
		t.Fatal("missing error location")
	}
}
