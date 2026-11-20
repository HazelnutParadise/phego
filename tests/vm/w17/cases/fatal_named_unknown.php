<?php
function f($a = 1, $b = 2) {
    echo "called ", $a, $b, "\n";
}
function say($x) {
    echo "say ", $x, "\n";
    return $x;
}
function wrap() {
    f(b: say("ok"));
    f(a: say("first"),
        nope: say("unknown value"),
        b: say("never"));
}
wrap();
echo "not reached\n";
