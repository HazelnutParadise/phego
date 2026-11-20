<?php
function ok() {
    static $n = 0;
    return ++$n;
}
function broken($x) {
    echo "enter ", $x, "\n";
    static $before = "initialized before the failure";
    static $v = [1, 2]
        + $x;
    return $v;
}
function wrap($x) {
    return broken($x);
}
echo ok(), ok(), "\n";
wrap(3);
echo "not reached\n";
