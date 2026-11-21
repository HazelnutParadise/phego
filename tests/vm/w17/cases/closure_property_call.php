<?php
// closure_property_call: property call syntax versus method call syntax
class A {
    public $f;
    public static $sf;
    public $nested;

    public function f() {
        return "method f";
    }
    public function __construct() {
        $this->f = function () {
            return "closure in property";
        };
    }
    public static function sm() {
        return "static m";
    }
}

A::$sf = function ($x) {
    return "static prop closure " . $x;
};
$o = new A();
echo "method_call: ", $o->f(), "\n";
echo "property_closure: ", ($o->f)(), "\n";
echo "static_prop_closure: ", (A::$sf)("arg"), "\n";
$cb = A::$sf;
echo "static_prop_copy: ", $cb("copy"), "\n";
echo "property_is_set: ", isset($o->f) ? "yes" : "no", "\n";

$o->f = function () {
    return "reassigned property closure";
};
echo "reassigned: ", ($o->f)(), "\n";
echo "method_still: ", $o->f(), "\n";

$o->nested = new stdClass();
$o->nested->fn = function ($y) {
    return "deep " . $y;
};
echo "nested_property: ", ($o->nested->fn)("x"), "\n";

$list = ["k" => function () { return "from array"; }];
echo "array_elem_closure: ", ($list["k"])(), "\n";
echo "static_method_still: ", A::sm(), "\n";
