package ir

import "testing"

func TestCompatibilityDeclarationReview(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		line               int
	}{
		{"promoted-readonly", "<?php abstract class A { protected abstract int $prop { get; set; } } class B extends A { function __construct(protected readonly int $prop) {} }", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::$prop::set)", 1},
		{"plain-readonly", "<?php abstract class A { protected abstract int $prop { get; set; } } class B extends A { protected readonly int $prop; }", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::$prop::set)", 1},
		{"promoted-mutable", "<?php abstract class A { protected abstract int $prop { get; set; } } class B extends A { function __construct(protected int $prop) {} }", "", 0},
		{"plain-mutable", "<?php abstract class A { protected abstract int $prop { get; set; } } class B extends A { protected int $prop; }", "", 0},
		{"abstract-missing", "<?php abstract class A { abstract function f(); } class B extends A {}", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::f)", 1},
		{"abstract-order", "<?php abstract class A { abstract function z(); abstract function a(); abstract function b(); abstract function c(); } class B extends A {}", "Class B contains 4 abstract methods and must therefore be declared abstract or implement the remaining methods (A::z, A::a, A::b, ...)", 1},
		{"abstract-intermediate", "<?php abstract class A { abstract function z(); abstract function a(); } abstract class B extends A { abstract function b(); function z(){} } class C extends B {}", "Class C contains 2 abstract methods and must therefore be declared abstract or implement the remaining methods (B::b, A::a)", 1},
		{"abstract-override", "<?php abstract class A { abstract function z(); abstract function a(); } abstract class B extends A { abstract function a(); } class C extends B {}", "Class C contains 2 abstract methods and must therefore be declared abstract or implement the remaining methods (B::a, A::z)", 1},
		{"abstract-implemented", "<?php abstract class A { abstract function z(); } class B extends A { function Z(){} } class C extends B {}", "", 0},
		{"abstract-hook-case", "<?php abstract class A { abstract public int $foo { get; } } class B extends A { public int $FOO; }", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::$foo::get)", 1},
		{"abstract-set-inherited", "<?php abstract class A { abstract public int $x {get;set;} } class B extends A { public int $x {get => 1;} }", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::$x::set)", 1},
		{"readonly-set-visibility", "<?php abstract class A { abstract public int $x {set;} } class B extends A { public readonly int $x; }", "Set access level of B::$x must be omitted (as in class A)", 1},
		{"abstract-hook-and-method", "<?php abstract class A { abstract public int $foo { get; set; } abstract function z(); abstract function a(); } class B extends A {}", "Class B contains 4 abstract methods and must therefore be declared abstract or implement the remaining methods (A::z, A::a, A::$foo::get, ...)", 1},
		{"final-static-property", "<?php class A { final public static int $x; } class B extends A { public int $x; }", "Cannot override final property A::$x", 1},
		{"abstract-get-inherited", "<?php abstract class A { abstract public int $x {get;set;} } class B extends A { public int $x {set {} } }", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::$x::get)", 1},
		{"abstract-hook-grandchild", "<?php abstract class A { abstract public int $x {get;set;} } abstract class B extends A { public int $x {get => 1;} } class C extends B {}", "", 0},
		{"abstract-hook-order", "<?php abstract class A { abstract public int $x {set;get;} } class B extends A {}", "Class B contains 2 abstract methods and must therefore be declared abstract or implement the remaining methods (A::$x::get, A::$x::set)", 1},
		{"abstract-both-plain", "<?php abstract class A { abstract public int $x {get;set;} } class B extends A { public int $x; }", "", 0},
		{"readonly-set-public", "<?php abstract class A { abstract public int $x {set;} } class B extends A { public public(set) readonly int $x; }", "Class B contains 1 abstract method and must therefore be declared abstract or implement the remaining method (A::$x::set)", 1},
		{"property-public-visibility", "<?php class A { public int $x; } class B extends A { protected int $x; }", "Access level to B::$x must be public (as in class A)", 1},
		{"hook-newclass-args", "<?php class A { public int $x = 1 { get { $f = new class($this->x) { function __construct($x) {} }; return 1; } } }", "", 0},
		{"final-private-ctor", "<?php class A { final private function __construct() {} } class B extends A { function __construct() {} }", "Cannot override final method A::__construct()", 1},
		{"constructor-prototype-visibility", "<?php abstract class Foo { abstract public function __construct(); } class Bar extends Foo { public function __construct(){} } class Baz extends Bar { protected function __construct(){} }", "Access level to Baz::__construct() must be public (as in class Foo)", 1},
		{"private-ctor", "<?php class A { private function __construct(int $x) {} } class B extends A { function __construct() {} }", "", 0},
		{"static-method", "<?php class A { static function f() {} } class B extends A { function f() {} }", "Cannot make static method A::f() non static in class B", 1},
		{"nonstatic-method", "<?php class A { function f() {} } class B extends A { static function f() {} }", "Cannot make non static method A::f() static in class B", 1},
		{"abstract-from-concrete", "<?php class A { function f() {} } abstract class B extends A { abstract function f(); }", "Cannot make non abstract method A::f() abstract in class B", 1},
		{"static-before-abstract", "<?php class A { static function f() {} } abstract class B extends A { abstract function f(); }", "Cannot make static method A::f() non static in class B", 1},
		{"final-before-static", "<?php class A { final static function f() {} } class B extends A { function f() {} }", "Cannot override final method A::f()", 1},
		{"static-property", "<?php class A { public static int $x; } class B extends A { public int $x; }", "Cannot redeclare static A::$x as non static B::$x", 1},
		{"nonstatic-property", "<?php class A { public int $x; } class B extends A { public static int $x; }", "Cannot redeclare non static A::$x as static B::$x", 1},
		{"constant-visibility", "<?php class A { public const X = 1; } class B extends A { private const X = 2; }", "Access level to B::X must be public (as in class A)", 1},
		{"constant-protected", "<?php class A { protected const X = 1; } class B extends A { private const X = 2; }", "Access level to B::X must be protected (as in class A) or weaker", 1},
		{"constant-wider", "<?php class A { protected const X = 1; } class B extends A { public const X = 2; }", "", 0},
		{"set-empty", "<?php class A { public int $x { set() {} } }", "set hook of property A::$x must accept exactly one parameters", 1},
		{"set-implicit", "<?php class A { public int $x { set {} } }", "", 0},
		{"set-many", "<?php class A { public int $x { set(int $value, int $other) {} } }", "set hook of property A::$x must accept exactly one parameters", 1},
		{"attr-array-unpack", "<?php #[A([...[1,2]])] class B {}", "", 0},
		{"attr-argument-unpack", "<?php #[A(...[1,2])] class B {}", "Cannot use unpacking in attribute argument list", 1},
		{"hook-closure", "<?php class A { public int $x = 1 { get { $f = function() { return $this->x; }; return 1; } } }", "Cannot specify default value for virtual hooked property A::$x", 1},
		{"hook-arrow", "<?php class A { public int $x = 1 { get { $f = fn() => $this->x; return 1; } } }", "Cannot specify default value for virtual hooked property A::$x", 1},
		{"hook-nested-class", "<?php class A { public int $x = 1 { get { $f = new class { function f() { return $this->x; } }; return 1; } } }", "Cannot specify default value for virtual hooked property A::$x", 1},
		{"hook-nullsafe", "<?php class A { public int $x = 1 { get => $this?->x; } }", "", 0},
		{"hook-case-sensitive", "<?php class A { public int $x = 1 { get => $this->X; } }", "Cannot specify default value for virtual hooked property A::$x", 1},
		{"hook-direct", "<?php class A { public int $x = 1 { get => $this->x; } }", "", 0},
		{"readonly-class-default", "<?php readonly class A { public int $x = 1; }", "Readonly property A::$x cannot have default value", 1},
		{"readonly-parent-explicit", "<?php readonly class A { public readonly int $x; } readonly class B extends A { public int $x; }", "", 0},
		{"readonly-child-explicit", "<?php readonly class A { public int $x; } readonly class B extends A { public readonly int $x; }", "", 0},
		{"attr-validator-before-target", "<?php #[AllowDynamicProperties, Override] readonly class A {}", "Cannot apply #[\\AllowDynamicProperties] to readonly class A", 1},
		{"attr-target-before-validator", "<?php #[Override, AllowDynamicProperties] readonly class A {}", "Attribute \"Override\" cannot target class (allowed targets: method, property)", 1},
		{"attr-delayed", "<?php #[AllowDynamicProperties, Override, DelayedTargetValidation] readonly class A {}", "", 0},
		{"attr-repeated-before-validator", "<?php #[AllowDynamicProperties, AllowDynamicProperties] readonly class A {}", "Attribute \"AllowDynamicProperties\" must not be repeated", 1},
		{"attr-expression-before-validator", "<?php #[AllowDynamicProperties, Foo($x)] readonly class A {}", "Constant expression contains invalid operations", 1},
		{"attr-interface-before-target", "<?php #[Attribute, Override] interface A {}", "Cannot apply #[\\Attribute] to interface A", 1},
		{"attr-deprecated-before-target", "<?php #[Deprecated, Override] class A {}", "Cannot apply #[\\Deprecated] to class A", 1},
		{"attr-abstract-before-target", "<?php #[Attribute, Override] abstract class A {}", "Cannot apply #[\\Attribute] to abstract class A", 1},
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
				if !ok || ce.Message != tc.want || ce.Line != tc.line {
					t.Fatalf("int%d: got %v, want line%d %s", width, err, tc.line, tc.want)
				}
			}
		})
	}
}
