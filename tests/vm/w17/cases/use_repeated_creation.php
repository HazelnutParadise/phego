<?php
// use_repeated_creation: one declaration executed several times gives independent closures
function makeScaler($factor) {
    return function ($n) use ($factor) {
        return $n * $factor;
    };
}
$double = makeScaler(2);
$triple = makeScaler(3);
echo "double_4: ", $double(4), "\n";
echo "triple_4: ", $triple(4), "\n";
echo "double_5: ", $double(5), "\n";

$list = [];
foreach ([1, 2, 3] as $n) {
    $list[] = function () use ($n) {
        return "n=" . $n;
    };
}
echo "list_0: ", $list[0](), "\n";
echo "list_2: ", $list[2](), "\n";
echo "distinct_objects: ", $list[0] === $list[1] ? "same" : "different", "\n";

$shared = 10;
$first = function () use ($shared) {
    return $shared;
};
$shared = 20;
$second = function () use ($shared) {
    return $shared;
};
echo "first: ", $first(), " second: ", $second(), "\n";

$made = [];
for ($i = 0; $i < 2; $i = $i + 1) {
    $made[] = makeScaler($i + 5);
}
echo "made_0: ", $made[0](2), "\n";
echo "made_1: ", $made[1](2), "\n";
echo "scaler_again: ", $double(10), "\n";
