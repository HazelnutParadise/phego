<?php
function f(...$a) {
    echo "called\n";
}
function say($x) {
    echo $x, "\n";
    return $x;
}
f(...[]);
f(say("before"),
    ...say(42),
    ...say("after"));
echo "not reached\n";
