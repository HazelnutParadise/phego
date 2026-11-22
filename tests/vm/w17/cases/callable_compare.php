<?php
function f($x) {
    return $x;
}

function g($x) {
    return $x;
}

class A {
    public function m() {
        return 1;
    }

    public static function sm() {
        return 2;
    }
}

$a = new A;
$b = new A;

$f1 = f(...);
$f2 = f(...);
echo "same_function_identity: ", $f1 === $f2 ? "same" : "different", "\n";
echo "same_function_equal: ", $f1 == $f2 ? "equal" : "not equal", "\n";

$g1 = g(...);
echo "different_function_equal: ", $f1 == $g1 ? "equal" : "not equal", "\n";

$m1 = $a->m(...);
$m2 = $a->m(...);
echo "same_object_method_identity: ", $m1 === $m2 ? "same" : "different", "\n";
echo "same_object_method_equal: ", $m1 == $m2 ? "equal" : "not equal", "\n";

$mb = $b->m(...);
echo "other_object_method_equal: ", $m1 == $mb ? "equal" : "not equal", "\n";

$s1 = A::sm(...);
$s2 = A::sm(...);
echo "static_method_equal: ", $s1 == $s2 ? "equal" : "not equal", "\n";
echo "static_method_identity: ", $s1 === $s2 ? "same" : "different", "\n";

$real = function ($x) {
    return $x;
};
echo "function_vs_real_closure: ", $f1 == $real ? "equal" : "not equal", "\n";
echo "method_vs_static_method: ", $m1 == $s1 ? "equal" : "not equal", "\n";
echo "real_closure_with_itself: ", $real == $real ? "equal" : "not equal", "\n";
