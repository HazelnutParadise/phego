<?php
$double = fn ($x) => $x * 2;
echo "double_int: ", $double(3), "\n";
echo "double_numeric_string: ", $double("4"), "\n";
$none = fn () => "none";
echo "no_params: ", $none(), "\n";
$sum3 = fn ($a, $b, $c) => $a + $b + $c;
echo "three_params: ", $sum3(1, 2, 3), "\n";
$arr = fn ($n) => [$n, $n + 1, $n + 2];
$got = $arr(10);
foreach ($got as $k => $v) {
    echo "arr_key_", $k, ": ", $v, "\n";
}
$greet = fn ($name) => "hello " . $name;
echo "string_return: ", $greet("phego"), "\n";
echo "iife_no_args: ", (fn () => 1)(), "\n";
echo "iife_args: ", (fn ($a, $b) => $a . "-" . $b)("x", "y"), "\n";
$adder = fn ($a) => fn ($b) => $a + $b;
$add5 = $adder(5);
echo "returns_arrow: ", $add5(7), "\n";
echo "chained_call: ", $adder(1)(2), "\n";
$isEven = fn ($n) => $n % 2 === 0;
echo "bool_true: ", $isEven(4) ? "yes" : "no", "\n";
echo "bool_false: ", $isEven(5) ? "yes" : "no", "\n";
$identity = fn ($v) => $v;
echo "null_return: ", $identity(null) === null ? "null" : "not null", "\n";
echo "float_return: ", $identity(2.5), "\n";
echo "is_closure: ", $double instanceof Closure ? "Closure" : "other", "\n";
echo "zero_args_string: ", (fn () => "z" . "w")(), "\n";
