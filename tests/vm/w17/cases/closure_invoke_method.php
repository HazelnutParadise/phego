<?php
// closure_invoke_method: calling Closure::__invoke explicitly
$f = function ($a, $b = 0) {
    return "a=$a b=$b";
};
echo "invoke_positional: ", $f->__invoke(1, 2), "\n";
echo "invoke_default: ", $f->__invoke(7), "\n";
$args = [3, 4];
echo "invoke_unpack: ", $f->__invoke(...$args), "\n";
echo "invoke_named: ", $f->__invoke(b: 9, a: 8), "\n";
$named = ["b" => "B2", "a" => "A2"];
echo "invoke_unpack_named: ", $f->__invoke(...$named), "\n";
echo "invoke_same_as_call: ", $f->__invoke(5) === $f(5) ? "same" : "different", "\n";

$inc = function (&$n, $step = 1) {
    $n = $n + $step;
    return $n;
};
$v = 10;
echo "invoke_ref_return: ", $inc->__invoke($v), "\n";
echo "invoke_ref_var: ", $v, "\n";
$inc->__invoke($v, 5);
echo "invoke_ref_after_step: ", $v, "\n";
echo "direct_call_ref: ", $inc($v), "\n";
echo "after_direct: ", $v, "\n";

class Adder {
    public function __invoke($x) {
        return $x + 100;
    }
}
$ad = new Adder();
echo "object_invoke: ", $ad->__invoke(1), "\n";
echo "object_invoke_direct: ", $ad(2), "\n";

$holder = [$f, "x"];
echo "closure_in_array_invoke: ", $holder[0]->__invoke("e", "f"), "\n";
