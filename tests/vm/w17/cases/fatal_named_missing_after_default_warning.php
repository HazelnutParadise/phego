<?php
const N = 2;
function f($opt = "3 pears" * N,
    $other = [1][N] ?? "x",
    $needed = "has default",
    $required_last = null) {
    echo "called ", $opt, "\n";
}
function g($first = "1 apple" + N, $second, $third = 3) {
    echo "not reached\n";
}
f(required_last: 1);
g(third: "t");
