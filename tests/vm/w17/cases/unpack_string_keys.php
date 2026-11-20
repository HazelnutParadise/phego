<?php
function abc($a = "A", $b = "B", $c = "C") {
    echo $a, $b, $c, "\n";
}
abc(...["c" => 3]);
abc(...["b" => 2, "a" => 1]);
abc(...[1], ...["c" => 3]);
abc(...[1, "c" => 3]);
abc(1, ...["b" => 2]);
abc(...[1, 2], ...["c" => 3]);
abc(...["a" => 1], ...["b" => 2], ...["c" => 3]);
abc(...["a" => 1], c: 9);
abc(...[1], c: 9);
abc(...[], b: 9);

$opts = ["b" => "bee"];
abc(...$opts);
$opts["a"] = "ay";
abc(...$opts);

function collect($first, ...$rest) {
    echo "first=", $first;
    foreach ($rest as $k => $v) {
        echo " ", $k, "=", $v;
    }
    echo "\n";
}
collect(...["first" => 1, "x" => 2, "y" => 3]);
collect(...[1, 2, "x" => 3]);
collect(1, ...[2, 3], ...["x" => 4, "y" => 5]);
collect(...["x" => 1, "first" => 2]);
collect(...[1, "10" => "numeric string key", "k" => "named"]);

function required($a, $b) {
    echo $a, "/", $b, "\n";
}
required(...["b" => "bee", "a" => "ay"]);
$pairs = [["b" => 1, "a" => 2], [3, "b" => 4]];
foreach ($pairs as $p) {
    required(...$p);
}
