<?php
function f($a = 1) {
    echo "called\n";
}
$s = "text";
$n = null;
f(...[2]);
f(1, ...$s);
