<?php
// use_mixed: parameters, by-value, by-reference and static state in one closure
$step = 2;
$total = 0;
$acc = function ($add, $mult = 1) use ($step, &$total) {
    static $calls = 0;
    $calls = $calls + 1;
    $total = $total + ($add + $step) * $mult;
    return "call=" . $calls . " total=" . $total;
};
echo "first: ", $acc(1), "\n";
echo "second: ", $acc(2, 10), "\n";
echo "third: ", $acc(0), "\n";
echo "outer_total: ", $total, "\n";
$step = 100;
echo "fourth: ", $acc(1), "\n";
echo "outer_total_after: ", $total, "\n";

$pick = function (&$slot, $label = "none") use (&$total) {
    $slot = $label . ":" . $total;
};
$target = "";
$pick($target);
echo "by_ref_param_default: ", $target, "\n";
$pick($target, "named");
echo "by_ref_param_given: ", $target, "\n";

$other = function ($x) use ($step) {
    static $seen = 0;
    $seen = $seen + 1;
    return "other " . $seen . " " . ($x + $step);
};
echo "other_1: ", $other(1), "\n";
echo "other_2: ", $other(1), "\n";
echo "acc_still_counts: ", $acc(5), "\n";
