<?php
function f($a = 1, $b = 2) {
    echo "called ", $a, $b, "\n";
}
function say($x) {
    echo "say\n";
    return $x;
}
$args = ["b" => "bee", "nope" => 1, "a" => "ay"];
f(...["b" => 5]);
f(...$args, ...say([]));
echo "not reached\n";
