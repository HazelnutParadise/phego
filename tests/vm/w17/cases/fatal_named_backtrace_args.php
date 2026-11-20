<?php
function thrower($a, $b = "bee", ...$rest) {
    return undefined_function_in_thrower();
}
function mid($x, $y = "why", $z = "zed") {
    return thrower(b: $x, a: $y, extra: $z, another: [1]);
}
function top() {
    return mid(z: "last", x: "first");
}
top();
