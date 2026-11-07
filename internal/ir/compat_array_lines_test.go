package ir

import (
	"errors"
	"testing"
)

func TestCompatibilityArrayDiagnosticLines(t *testing.T) {
	for _, tc := range []struct{ src, msg string }{
		{"<?php ([\n ...1\n])::class;", "Only arrays and Traversables can be unpacked, int given"},
		{"<?php ([\n []=>1\n])::class;", "Illegal offset type"},
	} {
		_, err := lowerSource(tc.src, 64)
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Line != 2 || ce.Message != tc.msg {
			t.Errorf("source=%q got %#v want line2 %s", tc.src, ce, tc.msg)
		}
	}
}

func TestCompatibilityEmptyArrayDiagnosticLines(t *testing.T) {
	for _, tc := range []struct {
		src  string
		line int
	}{
		{"<?php\n$a = [\n1,\n2,\n3,\n,\n5];", 5},
		{"<?php\n$a = [\n,\n1];", 3},
		{"<?php const X =\n[\n,\n1];", 1},
		{"<?php\n\nconst X =\n[\n,\n1];", 3},
		{"<?php\n1;\nconst X =\n[\n,\n1];", 3},
		{"<?php\nclass C {\nconst X =\n[\n,\n1];}", 3},
		{"<?php\nclass C {\npublic $x =\n[\n,\n1];}", 3},
		{"<?php\nfunction f(\n$x =\n[\n,\n1]){}", 3},
		{"<?php\nconst\nX =\n[\n,\n1];", 3},
		{"<?php\nconst X =\n1,\nY =\n[\n,\n1];", 2},
		{"<?php\nclass C {\npublic\nconst\nX =\n[\n,\n1];}", 5},
		{"<?php\nclass C {\npublic\nint\n$x =\n[\n,\n1];}", 4},
		{"<?php\nfunction f(\n#[A]\nint\n$x =\n[\n,\n1]){}", 4},
		{"<?php\nclass C {\npublic $x =\n1,\n$y =\n[\n,\n1];}", 3},
		{"<?php [\n 1 =>\n 2,\n ,\n 3];", 3},
		{"<?php [\n ...[1],\n ,\n 3];", 2},
	} {
		_, err := lowerSource(tc.src, 64)
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Line != tc.line || ce.Message != "Cannot use empty array elements in arrays" {
			t.Errorf("source=%q got %#v want line%d", tc.src, ce, tc.line)
		}
	}
}
