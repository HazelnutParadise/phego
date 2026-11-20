<?php
function depth($n) {
    static $level = 0, $max = 0;
    $level++;
    if ($level > $max) {
        $max = $level;
    }
    if ($n > 0) {
        depth($n - 1);
    }
    $level--;
    return $max . ":" . $level;
}
echo depth(4), "\n";
echo depth(2), "\n";
echo depth(6), "\n";

function memo_fib($n) {
    static $memo = [0, 1];
    static $hits = 0;
    if (isset($memo[$n])) {
        $hits++;
        return $memo[$n];
    }
    $memo[$n] = memo_fib($n - 1) + memo_fib($n - 2);
    return $memo[$n];
}
echo memo_fib(30), "\n";

function init_reenters($n) {
    echo "<enter ", $n, ">";
    static $v = helper($n);
    echo "<bound ", $v, ">";
    return $v;
}
function helper($n) {
    echo "<helper ", $n, ">";
    if ($n > 0) {
        init_reenters($n - 1);
    }
    return "value " . $n;
}
echo init_reenters(2), "\n";
echo init_reenters(9), "\n";

function mutual_a($n) {
    static $calls = 0;
    $calls++;
    return $n > 0 ? mutual_b($n - 1) : "a" . $calls;
}
function mutual_b($n) {
    static $calls = 0;
    $calls++;
    return $n > 0 ? mutual_a($n - 1) : "b" . $calls;
}
echo mutual_a(5), " ", mutual_a(4), " ", mutual_b(1), "\n";
