<?php
function f($a = 1, $b = 2) {
    echo "called ", $a, $b, "\n";
}
function say($x) {
    echo "say ", $x, "\n";
    return $x;
}
f(1, b: 5);
f(say("positional"),
    a: say("named a"),
    b: say("never"));
echo "not reached\n";
