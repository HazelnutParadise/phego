<?php
function f($x) {
    return "f:" . $x;
}

$c = function ($x) {
    return "closure:" . $x;
};
$same = $c(...);
echo "same_object: ", $same === $c ? "same" : "different", "\n";
echo "call_same: ", $same(1), "\n";

class Inv {
    public function __invoke($a, $b) {
        return "inv:" . $a . "," . $b;
    }
}

$o = new Inv;
$ci = $o(...);
echo "invoke_is_closure: ", $ci instanceof Closure ? "Closure" : "other", "\n";
echo "invoke_closure_call: ", $ci(1, 2), "\n";
echo "direct_invoke_method: ", $o->__invoke(3, 4), "\n";
echo "invoke_object_call: ", $o(5, 6), "\n";

$cf = f(...);
echo "fcc_is_closure: ", $cf instanceof Closure ? "yes" : "no", "\n";
echo "fcc_call: ", $cf("g"), "\n";
$cf2 = $cf(...);
echo "closure_of_closure_same: ", $cf2 === $cf ? "same" : "different", "\n";
$ci2 = $ci(...);
echo "invoke_closure_of_closure_same: ", $ci2 === $ci ? "same" : "different", "\n";
echo "invoke_closure_again: ", $ci2(7, 8), "\n";
