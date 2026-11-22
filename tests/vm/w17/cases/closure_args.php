<?php
// closure_args: named arguments, unpacking, variadics, extra args, constant defaults
$f = function ($a, $b = 2, $c = 3) {
    return "a=$a b=$b c=$c";
};
echo "named_any_order: ", $f(c: 9, a: 1), "\n";
echo "named_skip_default: ", $f(1, c: 7), "\n";
$args = [4, 5, 6];
echo "unpack_list: ", $f(...$args), "\n";
$named = ["c" => 30, "a" => 10];
echo "unpack_string_keys: ", $f(...$named), "\n";
echo "unpack_after_positional: ", $f(7, ...["c" => 70]), "\n";

$collect = function ($first, ...$rest) {
    echo "first=$first\n";
    foreach ($rest as $k => $v) {
        echo "rest[$k]=$v\n";
    }
    return "done";
};
echo "variadic_none: ", $collect("only"), "\n";
echo "variadic_values: ", $collect("p", "q", "r"), "\n";
echo "variadic_unpack: ", $collect(...[1, 2, 3]), "\n";
echo "variadic_named: ", $collect("n", extra: "E", more: "M"), "\n";

$one = function ($x) {
    return "x=$x";
};
echo "extra_args_ignored: ", $one(1, 2, 3), "\n";

const BASE = 4;
$sized = function ($n = BASE * 2, $shift = 1 << 3, $list = [1, 2, 3], $name = "dflt" . "-x") {
    return "n=$n shift=$shift list=" . $list[2] . " name=$name";
};
echo "const_defaults: ", $sized(), "\n";
echo "const_default_partial: ", $sized(1), "\n";
echo "const_default_named: ", $sized(name: "given"), "\n";
