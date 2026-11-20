<?php
function f($a = 1,
    $b = "x" . MISSING_IN_DEFAULT,
    $c = 3, ...$rest) {
    echo "called\n";
}
function caller($n) {
    f("pos", "b given",
        c: [1, 2],
        more: 5.0);
    f(c: "see", extra: "named extra", a: "first");
}
function outer(...$args) {
    caller(...$args);
}
outer(n: 1);
