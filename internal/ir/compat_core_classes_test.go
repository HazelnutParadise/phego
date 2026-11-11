package ir

import "testing"

// Native PHP 8.5.12 lint confirms final-class diagnostics and the unresolved
// variance controls. These metadata checks do not execute built-in methods.
func TestCompatibilityCoreClassBinding(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"generator-final", "<?php class Child extends Generator {}", "Class Child cannot extend final class Generator"},
		{"weakreference-final", "<?php class Child extends WeakReference {}", "Class Child cannot extend final class WeakReference"},
		{"weakmap-final", "<?php class Child extends WeakMap {}", "Class Child cannot extend final class WeakMap"},
		{"generator-unknown", "<?php class Child extends Generator { function throw(Missing $exception): mixed {} }", ""},
		{"weakreference-unknown", "<?php class Child extends WeakReference { function get(): Missing {} }", ""},
		{"weakmap-unknown", "<?php class Child extends WeakMap { function offsetGet(Missing $object): mixed {} }", "Class Child cannot extend final class WeakMap"},
		{"generator-compatible", "<?php class Child extends Generator { function throw(object $exception): mixed {} }", "Class Child cannot extend final class Generator"},
		{"weakreference-compatible", "<?php class Child extends WeakReference { function get(): ?object {} }", "Class Child cannot extend final class WeakReference"},
		{"weakmap-compatible", "<?php class Child extends WeakMap { function count(): int {} }", "Class Child cannot extend final class WeakMap"},
		{"local-cannot-shadow", "<?php class Generator {} class Child extends Generator {}", "Class Child cannot extend final class Generator"},
		{"weakreference-negative", "<?php class Child extends WeakReference { function get(): int {} }", "Class Child cannot extend final class WeakReference"},
		{"generator-constructor", "<?php class Child extends Generator { function __construct(Missing $arg) {} }", "Class Child cannot extend final class Generator"},
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
