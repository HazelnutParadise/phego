<?php
function total($first = 0, ...$rest) {
    if ($rest === []) {
        return $first;
    }
    return $first + total(...$rest);
}
echo total(), " ", total(5), " ", total(1, 2, 3, 4, 5), "\n";

function join_with($sep, ...$parts) {
    $out = "";
    foreach ($parts as $i => $p) {
        if ($i > 0) {
            $out .= $sep;
        }
        $out .= $p;
    }
    return $out;
}
echo join_with("-", "a", "b", "c"), "|", join_with("-"), "|", join_with("-", "one"), "\n";

function forward(...$args) {
    return join_with(...$args);
}
echo forward(", ", 1, 2, 3), "\n";

function count_args(...$a) {
    $n = 0;
    foreach ($a as $v) {
        $n++;
    }
    return $n;
}
function nest(...$a) {
    return count_args(...$a, ...$a) . "/" . count_args($a, ...$a);
}
echo nest(1, 2, 3), "\n";

function reverse(...$items) {
    $out = [];
    foreach ($items as $v) {
        $out = prepend($v, ...$out);
    }
    return $out;
}
function prepend($v, ...$tail) {
    $r = [$v];
    foreach ($tail as $t) {
        $r[] = $t;
    }
    return $r;
}
foreach (reverse(1, 2, 3) as $v) {
    echo $v;
}
echo "\n";

function many(...$v) {
    $n = 0;
    foreach ($v as $x) {
        $n += $x;
    }
    return $n;
}
$big = [];
for ($i = 1; $i <= 300; $i++) {
    $big[] = $i;
}
echo many(...$big), "\n";
