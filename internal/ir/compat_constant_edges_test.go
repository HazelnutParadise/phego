package ir

import (
	"errors"
	"testing"
)

// These are php -n -l observations, not the results of executing ::class.
func TestCompatibilityConstantEdges(t *testing.T) {
	for _, tc := range []struct{ expression, kind string }{
		{`true+1`, "int"}, {`null+1`, "int"}, {`-true`, "int"}, {`+false`, "int"},
		{`~1.5`, ""}, {`1.5&2`, ""}, {`"1"&"2"`, "string"}, {`"a"|"b"`, "string"},
		{`2**-1`, "float"}, {`2**1025`, "float"}, {`0**1025`, "int"}, {`1**1025`, "int"},
		{`1e300*1e300`, "float"}, {`"1e999"+1`, "float"}, {`"\u{a0}1"+1`, ""},
		{`(2**-1)<1`, "true"}, {`"0"==null`, "false"}, {`""==null`, "true"},
		{`"foo"==0`, "false"}, {`"10"<"2"`, "false"}, {`"10"<2`, "false"},
		{`true&&false`, "false"}, {`false||true`, "true"}, {`true xor false`, "true"},
		{`false??true`, ""},
		{`"9223372036854775808" == "9223372036854775809"`, "false"},
		{`(1e20 . "") === "1.0E+20"`, "true"}, {`1e20 > "1.0E+20x"`, "false"},
		{`("AB"^"A")==="\0"`, "true"}, {`(~"\xc3\xa9")==="\x3c\x56"`, "true"},
		{`(0**0)==1`, "true"}, {`null < -1`, "true"}, {`null > -1`, "false"},
		{`"1.2345678901235" === (1.234567890123456 . "")`, "true"},
		{`(-1*(-9223372036854775807-1))>0`, "true"},
		{`((-9223372036854775807-1)*-1)>0`, "true"},
	} {
		t.Run(tc.expression, func(t *testing.T) {
			_, err := lowerSource("<?php ("+tc.expression+")::class;", 64)
			if tc.kind == "" {
				if err != nil {
					t.Fatalf("PHP compiles this expression: %v", err)
				}
				return
			}
			var ce *CompileError
			want := `Cannot use "::class" on ` + tc.kind
			if !errors.As(err, &ce) || ce.Line != 1 || ce.Message != want {
				t.Fatalf("error=%v, want line1 %s", err, want)
			}
		})
	}
}
