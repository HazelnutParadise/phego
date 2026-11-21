<?php
class A {
    public static function sm($x) {
        return "sm:" . $x;
    }

    public function m($x) {
        return "m:" . $x;
    }
}

$o = new A;
$f1 = [$o, 'm'](...);
echo "object_pair_fcc: ", $f1(1), "\n";

$f2 = ['A', 'sm'](...);
echo "class_pair_fcc: ", $f2(2), "\n";

$arr = [$o, 'm'];
$f3 = $arr(...);
echo "variable_pair_fcc: ", $f3(3), "\n";

echo "object_pair_plain: ", [$o, 'm'](4), "\n";
echo "class_pair_plain: ", ['A', 'sm'](5), "\n";
echo "variable_pair_plain: ", $arr(6), "\n";

$sarr = ['A', 'sm'];
echo "variable_static_pair_plain: ", $sarr(7), "\n";

echo "static_string_plain: ", 'A::sm'(8), "\n";
$f4 = 'A::sm'(...);
echo "static_string_fcc: ", $f4(9), "\n";

$f5 = [$o, 'm'](...);
echo "same_target_different_closures: ", $f5 === $f1 ? "same" : "different", "\n";
echo "fcc_from_pair_is_closure: ", $f5 instanceof Closure ? "yes" : "no", "\n";
echo "pair_fcc_call_again: ", $f5(10), "\n";
