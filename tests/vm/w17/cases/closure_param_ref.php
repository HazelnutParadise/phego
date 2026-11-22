<?php
// closure_param_ref: by-reference parameters of closures
$bump = function (&$n) {
    $n = $n + 1;
};
$x = 1;
$bump($x);
$bump($x);
echo "ref_param_modifies: ", $x, "\n";

$push = function (array &$list, $item) {
    $list[] = $item;
};
$items = ["first"];
$push($items, "second");
$push($items, "third");
echo "ref_array_append: ", $items[1], " ", $items[2], "\n";

$setElem = function (&$arr, $k, $v) {
    $arr[$k] = $v;
};
$data = [];
$setElem($data, "key", "val");
echo "ref_array_key: ", $data["key"], "\n";

$maybe = function (&$out = null) {
    $out = "set-by-default-call";
    return "returned";
};
echo "ref_default_no_arg: ", $maybe(), "\n";
$maybe($notYet);
echo "undefined_arg_created: ", $notYet, "\n";

$swap = function (&$a, &$b) {
    $t = $a;
    $a = $b;
    $b = $t;
};
$p = "left";
$q = "right";
$swap($p, $q);
echo "swap: ", $p, " ", $q, "\n";
$pair = ["m", "n"];
$swap($pair[0], $pair[1]);
echo "swap_elements: ", $pair[0], " ", $pair[1], "\n";

$fill = function (&...$refs) {
    $refs[0] = "A";
    $refs[1] = "B";
};
$one = "";
$two = "";
$fill($one, $two);
echo "by_ref_variadic: ", $one, $two, "\n";

$addAll = function (&$target, ...$nums) {
    foreach ($nums as $num) {
        $target = $target + $num;
    }
    return $target;
};
$sum = 0;
echo "ref_variadic_values: ", $addAll($sum, 1, 2, 3), "\n";
echo "sum_after: ", $sum, "\n";
