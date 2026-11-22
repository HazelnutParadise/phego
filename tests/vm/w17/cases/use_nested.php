<?php
// use_nested: a nested closure captures what the outer closure captured
$base = "B";
$outer = function ($p) use ($base) {
    $inner = function ($q) use ($base, $p) {
        return $base . $p . $q;
    };
    return $inner("q");
};
echo "chain_value: ", $outer("p"), "\n";

$counter = 0;
$outerRef = function () use (&$counter) {
    $inner = function () use (&$counter) {
        $counter = $counter + 1;
        return $counter;
    };
    return $inner() + $inner();
};
echo "ref_chain_sum: ", $outerRef(), "\n";
echo "ref_chain_counter: ", $counter, "\n";

$seed = 10;
$byValOuter = function () use ($seed) {
    $inner = function () use (&$seed) {
        $seed = $seed + 1;
        return $seed;
    };
    return $inner();
};
echo "inner_ref_of_outer_val: ", $byValOuter(), "\n";
echo "inner_ref_second_call: ", $byValOuter(), "\n";
echo "outer_seed: ", $seed, "\n";

$a1 = 1;
$l1 = function () use ($a1) {
    return function () use ($a1) {
        return function () use ($a1) {
            return $a1 + 100;
        };
    };
};
echo "three_level: ", $l1()()(), "\n";

$tag = "X";
$arrowInside = function () use ($tag) {
    return fn() => $tag . "!";
};
echo "arrow_in_outer: ", $arrowInside()(), "\n";
