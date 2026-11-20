<?php
function f($a = 1) {
    echo "called\n";
}
function caller() {
    f(...$nothing);
}
f(...[]);
caller();
