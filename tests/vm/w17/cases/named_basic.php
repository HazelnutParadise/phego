<?php
function abc($a, $b = "B", $c = "C") {
    echo $a, "|", $b, "|", $c, "\n";
}
abc(a: 1);
abc(b: 2, a: 1);
abc(c: 3, b: 2, a: 1);
abc(1, c: 3);
abc(1, 2, c: 3);
abc(1, b: 2);
abc(1, c: 3, b: 2);
abc(a: 1, b: 2, c: 3);
abc(a: 1, c: 3);

$x = "var";
abc(a: $x, c: $x . "!");
abc(c: [1, 2][1], a: $x === "var" ? "yes" : "no");

function later_declared_caller() {
    declared_later(second: "2nd", first: "1st");
    declared_later(first: "only");
}
later_declared_caller();
function declared_later($first, $second = "default second") {
    echo $first, " ", $second, "\n";
}

function returns($value, $times = 2) {
    return $value * $times;
}
echo returns(times: 3, value: 5), " ", returns(value: 4), "\n";
echo returns(value: returns(times: 10, value: 1), times: returns(value: 1)), "\n";

function keywords($array = 1, $function = 2, $list = 3, $class = 4) {
    echo $array, $function, $list, $class, "\n";
}
keywords(class: "c", list: "l");
keywords(function: "f", array: "a");

function recurse($n, $acc = "") {
    if ($n === 0) {
        return $acc;
    }
    return recurse(acc: $acc . $n, n: $n - 1);
}
echo recurse(n: 5), "\n";

function case_param($Value = "default", $value = "lower default") {
    echo $Value, " ", $value, "\n";
}
case_param(value: "lower");
case_param(Value: "upper");
