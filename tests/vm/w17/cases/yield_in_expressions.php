<?php
function gen() {
    $arr = [yield 1, yield 2];
    echo "arr: ", $arr[0], ",", $arr[1], "\n";
    $sum = (yield 3) + (yield 4);
    echo "sum: $sum\n";
    $t = (yield 5) ? "yes" : "no";
    echo "t: $t\n";
    echo "call: ", id(yield 6), "\n";
}
function id($x) { return $x; }
$g = gen();
foreach (['a', 'b', 10, 20, true, 'c'] as $send) { echo "got ", $g->current(), "\n"; $g->send($send); }
