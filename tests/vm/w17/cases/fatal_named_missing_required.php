<?php
function f($a,
    $b,
    $c = "c") {
    echo "called\n";
}
function caller() {
    f(1, 2);
    f(a: 1, b: 2);
    echo "before\n";
    f(
        c: "three",
        b: "two"
    );
}
caller();
