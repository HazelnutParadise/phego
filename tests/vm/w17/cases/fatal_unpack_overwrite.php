<?php
function f($a = 1, $b = 2) {
    echo "called ", $a, $b, "\n";
}
function wrap() {
    f(...[1, 2]);
    f(...[1], ...["b" => 2]);
    f(...[1],
        ...["a" => 2]);
}
wrap();
echo "not reached\n";
