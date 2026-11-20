<?php
function f($a, $b, $c) {
    echo "called\n";
}
function caller($x) {
    f(a: $x,
        b: 2);
}
caller("x");
