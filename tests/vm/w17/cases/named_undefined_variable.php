<?php
function abc($a = "-", $b = "-", $c = "-") {
    echo $a === null ? "null" : $a, $b === null ? "null" : $b, $c === null ? "null" : $c, "\n";
}
abc(b: $undefined_b);
abc(c: $undefined_c,
    a: $undefined_a);
abc(1, c: $list["missing"]);
$list = ["k" => "v"];
abc(c: $list["nope"], a: $list["k"]);

function refs(&$x, $y = "y") {
    echo $x === null ? "null" : $x, " ", $y, "\n";
    $x = "created";
}
refs(y: $undefined_y, x: $created);
echo $created, "\n";

function variadic(...$v) {
    foreach ($v as $k => $x) {
        echo $k, "=", $x === null ? "null" : $x, " ";
    }
    echo "\n";
}
variadic(a: $nope1, b: $nope2);
variadic($nope3, n: $nope4);
