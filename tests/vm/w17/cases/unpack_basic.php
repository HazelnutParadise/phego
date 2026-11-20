<?php
function three($a, $b = "b-default", $c = "c-default") {
    echo $a, " ", $b, " ", $c, "\n";
}
$args = [1, 2, 3];
three(...$args);
three(...[1, 2]);
three(...[1]);
three("x", ...$args);
three("x", ...["y"]);
three(...[1], ...[2], ...[3]);
three(...[], ...[1, 2, 3], ...[]);
three(1, 2, ...[]);
three(...$args, ...[4, 5]);

$keyed = [5 => "five", 2 => "two", 9 => "nine"];
three(...$keyed);

function count_them(...$v) {
    $n = 0;
    foreach ($v as $k => $x) {
        echo $k, "=", $x, " ";
        $n++;
    }
    echo "(", $n, ")\n";
}
count_them(...$keyed);
count_them(...$keyed, ...$args);
count_them(0, ...$args, ...$args);
count_them(...[]);

function make() {
    echo "[make]";
    return ["m1", "m2"];
}
three(...make());
three(...make(), ...make());

$nested = [[1, 2], [3, 4]];
three(...$nested[1]);
three(...$nested[0], ...$nested[1]);

$ref_holder = [10, 20];
$alias = &$ref_holder;
three(...$alias);

function modifies($a, $b) {
    $a = "changed";
    $b = "changed";
}
$plain = ["keep a", "keep b"];
modifies(...$plain);
echo $plain[0], " ", $plain[1], "\n";

echo three(...["ret"]) === null ? "null result" : "other", "\n";
