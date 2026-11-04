package phego

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCompileHeredocRetainsSourceAnchor(t *testing.T) {
	e, err := NewEngine(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close(context.Background())
	_, err = e.Compile(context.Background(), "body.php", "<?php\n<<<X\n{$a + }\n X;")
	var pe *PHPError
	if !errors.As(err, &pe) || pe.Kind != ParseError || pe.File != "body.php" || pe.Line != 4 || !strings.Contains(pe.Message, "Invalid body indentation level") {
		t.Fatalf("compile error = %#v", err)
	}
}
