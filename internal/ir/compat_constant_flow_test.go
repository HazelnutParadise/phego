package ir

import "testing"

// PHP 8.5.12 lint checks constants at each completed subexpression. These
// controls distinguish compile-time short circuit from a runtime condition.
func TestCompatibilityConstantFlow(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"expression-empty-array-and", "<?php [] && [...1];", ""},
		{"assignment-empty-array-and", "<?php $a=[] && [...1];", ""},
		{"return-empty-array-and", "<?php return [] && [...1];", ""},
		{"if-empty-array-and", "<?php if ([] && [...1]) {}", ""},
		{"function-empty-array-and", "<?php function f(){return [] && [...1];}", ""},
		{"expression-folded-int-and", "<?php (1-1) && [...1];", ""},
		{"assignment-folded-int-and", "<?php $a=(1-1) && [...1];", ""},
		{"return-folded-int-and", "<?php return (1-1) && [...1];", ""},
		{"if-folded-int-and", "<?php if ((1-1) && [...1]) {}", ""},
		{"function-folded-int-and", "<?php function f(){return (1-1) && [...1];}", ""},
		{"expression-nonempty-array-or", "<?php [-1] || [...1];", ""},
		{"assignment-nonempty-array-or", "<?php $a=[-1] || [...1];", ""},
		{"return-nonempty-array-or", "<?php return [-1] || [...1];", ""},
		{"if-nonempty-array-or", "<?php if ([-1] || [...1]) {}", ""},
		{"function-nonempty-array-or", "<?php function f(){return [-1] || [...1];}", ""},
		{"expression-invalid-array-key", "<?php [[]=>1];", "Illegal offset type"},
		{"assignment-invalid-array-key", "<?php $a=[[]=>1];", "Illegal offset type"},
		{"return-invalid-array-key", "<?php return [[]=>1];", "Illegal offset type"},
		{"if-invalid-array-key", "<?php if ([[]=>1]) {}", "Illegal offset type"},
		{"function-invalid-array-key", "<?php function f(){return [[]=>1];}", "Illegal offset type"},
		{"expression-unknown-plus-invalid-key", "<?php ($x + [[]=>1])::class;", "Illegal offset type"},
		{"assignment-unknown-plus-invalid-key", "<?php $a=($x + [[]=>1])::class;", "Illegal offset type"},
		{"return-unknown-plus-invalid-key", "<?php return ($x + [[]=>1])::class;", "Illegal offset type"},
		{"if-unknown-plus-invalid-key", "<?php if (($x + [[]=>1])::class) {}", "Illegal offset type"},
		{"function-unknown-plus-invalid-key", "<?php function f(){return ($x + [[]=>1])::class;}", "Illegal offset type"},
		{"expression-known-plus-invalid-key", "<?php ([null=>1] + [[]=>1])::class;", "Illegal offset type"},
		{"assignment-known-plus-invalid-key", "<?php $a=([null=>1] + [[]=>1])::class;", "Illegal offset type"},
		{"return-known-plus-invalid-key", "<?php return ([null=>1] + [[]=>1])::class;", "Illegal offset type"},
		{"if-known-plus-invalid-key", "<?php if (([null=>1] + [[]=>1])::class) {}", "Illegal offset type"},
		{"function-known-plus-invalid-key", "<?php function f(){return ([null=>1] + [[]=>1])::class;}", "Illegal offset type"},
		{"expression-illegal-key-first", "<?php ([[]=>1] + [...1])::class;", "Illegal offset type"},
		{"assignment-illegal-key-first", "<?php $a=([[]=>1] + [...1])::class;", "Illegal offset type"},
		{"return-illegal-key-first", "<?php return ([[]=>1] + [...1])::class;", "Illegal offset type"},
		{"if-illegal-key-first", "<?php if (([[]=>1] + [...1])::class) {}", "Illegal offset type"},
		{"function-illegal-key-first", "<?php function f(){return ([[]=>1] + [...1])::class;}", "Illegal offset type"},
		{"expression-invalid-key-dynamic-value", "<?php [[]=>$x];", ""},
		{"assignment-invalid-key-dynamic-value", "<?php $a=[[]=>$x];", ""},
		{"return-invalid-key-dynamic-value", "<?php return [[]=>$x];", ""},
		{"if-invalid-key-dynamic-value", "<?php if ([[]=>$x]) {}", ""},
		{"function-invalid-key-dynamic-value", "<?php function f(){return [[]=>$x];}", ""},
		{"expression-unknown-left-keeps-right", "<?php $x && [...1];", "Only arrays and Traversables can be unpacked, int given"},
		{"assignment-unknown-left-keeps-right", "<?php $a=$x && [...1];", "Only arrays and Traversables can be unpacked, int given"},
		{"return-unknown-left-keeps-right", "<?php return $x && [...1];", "Only arrays and Traversables can be unpacked, int given"},
		{"if-unknown-left-keeps-right", "<?php if ($x && [...1]) {}", "Only arrays and Traversables can be unpacked, int given"},
		{"function-unknown-left-keeps-right", "<?php function f(){return $x && [...1];}", "Only arrays and Traversables can be unpacked, int given"},
		{"expression-nonzero-left-keeps-right", "<?php [1] && [...1];", "Only arrays and Traversables can be unpacked, int given"},
		{"assignment-nonzero-left-keeps-right", "<?php $a=[1] && [...1];", "Only arrays and Traversables can be unpacked, int given"},
		{"return-nonzero-left-keeps-right", "<?php return [1] && [...1];", "Only arrays and Traversables can be unpacked, int given"},
		{"if-nonzero-left-keeps-right", "<?php if ([1] && [...1]) {}", "Only arrays and Traversables can be unpacked, int given"},
		{"function-nonzero-left-keeps-right", "<?php function f(){return [1] && [...1];}", "Only arrays and Traversables can be unpacked, int given"},
		{"expression-false-class", "<?php ([] && [...1])::class;", "Cannot use \"::class\" on false"},
		{"assignment-false-class", "<?php $a=([] && [...1])::class;", "Cannot use \"::class\" on false"},
		{"return-false-class", "<?php return ([] && [...1])::class;", "Cannot use \"::class\" on false"},
		{"if-false-class", "<?php if (([] && [...1])::class) {}", "Cannot use \"::class\" on false"},
		{"function-false-class", "<?php function f(){return ([] && [...1])::class;}", "Cannot use \"::class\" on false"},
		{"expression-float-not", "<?php (~1.0)::class;", "Cannot use \"::class\" on int"},
		{"assignment-float-not", "<?php $a=(~1.0)::class;", "Cannot use \"::class\" on int"},
		{"return-float-not", "<?php return (~1.0)::class;", "Cannot use \"::class\" on int"},
		{"if-float-not", "<?php if ((~1.0)::class) {}", "Cannot use \"::class\" on int"},
		{"function-float-not", "<?php function f(){return (~1.0)::class;}", "Cannot use \"::class\" on int"},
		{"expression-lossy-float-not", "<?php (~1.5)::class;", ""},
		{"assignment-lossy-float-not", "<?php $a=(~1.5)::class;", ""},
		{"return-lossy-float-not", "<?php return (~1.5)::class;", ""},
		{"if-lossy-float-not", "<?php if ((~1.5)::class) {}", ""},
		{"function-lossy-float-not", "<?php function f(){return (~1.5)::class;}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, width := range []int{32, 64} {
				_, err := lowerSource(tc.source, width)
				if tc.want == "" {
					if err != nil {
						t.Fatalf("int%d: %v", width, err)
					}
					continue
				}
				ce, ok := err.(*CompileError)
				if !ok || ce.Message != tc.want || ce.Line != 1 {
					t.Fatalf("int%d: got %v, want %s", width, err, tc.want)
				}
			}
		})
	}
}
