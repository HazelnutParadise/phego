<?php
function f(...$a) {
    echo "called\n";
}
function say($x) {
    echo "say\n";
    return $x;
}
f(...["k" => 1]);
f(...["k" => 1, 2], ...say([3]));
echo "not reached\n";
