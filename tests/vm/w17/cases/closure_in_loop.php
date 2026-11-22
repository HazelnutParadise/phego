<?php
// closure_in_loop: closures created inside loops
$byValue = [];
for ($i = 0; $i < 3; $i = $i + 1) {
    $byValue[] = function () use ($i) {
        return "i=" . $i;
    };
}
echo "for_by_value_0: ", $byValue[0](), "\n";
echo "for_by_value_1: ", $byValue[1](), "\n";
echo "for_by_value_2: ", $byValue[2](), "\n";

$byRef = [];
for ($j = 0; $j < 3; $j = $j + 1) {
    $byRef[] = function () use (&$j) {
        return "j=" . $j;
    };
}
echo "for_by_ref_0: ", $byRef[0](), "\n";
echo "for_by_ref_2: ", $byRef[2](), "\n";
echo "j_after_loop: ", $j, "\n";

$letters = ["a", "b", "c"];
$fs = [];
foreach ($letters as $k => $v) {
    $fs[] = function () use ($k, $v) {
        return $k . ":" . $v;
    };
}
foreach ($fs as $idx => $fn) {
    echo "foreach_", $idx, ": ", $fn(), "\n";
}

$doubles = [];
foreach ([1, 2, 3] as $n) {
    $doubles[] = function () use ($n) {
        return $n * 2;
    };
}
echo "double_first: ", $doubles[0](), "\n";
echo "double_last: ", $doubles[2](), "\n";

$refs = [];
foreach ([10, 20] as $x) {
    $refs[] = function () use (&$x) {
        return $x;
    };
}
echo "foreach_ref_all: ", $refs[0](), " ", $refs[1](), "\n";

$later = [];
for ($k2 = 1; $k2 <= 3; $k2 = $k2 + 1) {
    $later["k$k2"] = function ($mul) use ($k2) {
        return $k2 * $mul;
    };
}
foreach ($later as $name => $fn2) {
    echo $name, " => ", $fn2(10), "\n";
}
