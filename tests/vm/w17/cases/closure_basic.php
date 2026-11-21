<?php
// closure_basic: define, assign, call, parameters, defaults, returns
$add = function ($a, $b = 10) {
    return $a + $b;
};
echo "assign_call: ", $add(1, 2), "\n";
echo "default_param: ", $add(5), "\n";
echo "default_overridden: ", $add(5, 0), "\n";

$pair = function ($s) {
    return ["left" => $s . "L", "right" => $s . "R"];
};
$got = $pair("x");
echo "return_array: ", $got["left"], " ", $got["right"], "\n";

$early = function ($n) {
    if ($n < 0) {
        return "negative";
    }
    return "non-negative";
};
echo "early_return_neg: ", $early(-1), "\n";
echo "early_return_pos: ", $early(1), "\n";

$noValue = function () {
    $unused = 1;
};
echo "no_return_is_null: ", $noValue() === null ? "null" : "other", "\n";

$outer = function ($x) {
    $inner = function ($y) use ($x) {
        $deeper = function ($z) use ($x, $y) {
            return $x . "+" . $y . "+" . $z;
        };
        return $deeper("c");
    };
    return $inner("b");
};
echo "nested_three: ", $outer("a"), "\n";

echo "iife_no_args: ", (function () { return "iife-value"; })(), "\n";
echo "iife_args: ", (function ($p, $q) { return $p . "-" . $q; })("a", "b"), "\n";

$arr = [function ($n) { return "arr:" . $n; }];
echo "array_elem: ", $arr[0]("x"), "\n";
echo "array_elem_int: ", $arr[0](42), "\n";

class Holder {
    public $p;
}
$o = new Holder();
$o->p = function ($n) { return "prop:" . $n; };
echo "object_prop: ", ($o->p)("y"), "\n";

$f = $o->p;
echo "copy_then_call: ", $f("z"), "\n";
