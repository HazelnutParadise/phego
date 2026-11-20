<?php
function f($a = 1, $b = 2) {
    echo "called ", $a, $b, "\n";
}
function caller() {
    f(b: 1, a: 2);
    f(b: 1,
        b: $undefined_not_read);
}
caller();
