<?php
function f($n) {
    echo "call ", $n, "\n";
    static $v = "x" .
        MISSING_CONSTANT;
    return $v;
}
function g() {
    return f(1);
}
echo g();
