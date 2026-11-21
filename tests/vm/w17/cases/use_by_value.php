<?php
// use_by_value: captured values are copies taken at creation
$x = 1;
$byVal = function () use ($x) {
    return "x=" . $x;
};
$x = 2;
echo "after_outer_change: ", $byVal(), "\n";
echo "outer_x: ", $x, "\n";

$mutate = function () use ($x) {
    $x = $x + 100;
    return $x;
};
echo "inside_assign_1: ", $mutate(), "\n";
echo "inside_assign_2: ", $mutate(), "\n";
echo "outer_still: ", $x, "\n";

$counter = 0;
$incr = function () use ($counter) {
    $counter = $counter + 1;
    return $counter;
};
echo "incr_1: ", $incr(), "\n";
echo "incr_2: ", $incr(), "\n";
echo "counter_outer: ", $counter, "\n";

$name = "first";
$greet = function () use ($name) {
    return "hello " . $name;
};
$name = "second";
echo "greet: ", $greet(), "\n";
$greet2 = function () use ($name) {
    return $name;
};
echo "greet2: ", $greet2(), "\n";

function make($v) {
    return function () use ($v) {
        return "made:" . $v;
    };
}
$m = make("A");
echo "made_A: ", $m(), "\n";

$keep = "kept";
$k = function () use ($keep) {
    return $keep;
};
unset($keep);
echo "after_unset_outer: ", $k(), "\n";

$late = 5;
$lateF = function () use ($late) {
    return $late * 2;
};
$late = $late + 1;
echo "late_value: ", $lateF(), "\n";
echo "late_outer: ", $late, "\n";
