<?php
function collect($first = "F", ...$rest) {
    echo "first=", $first, ";";
    foreach ($rest as $k => $v) {
        echo " ", $k, "=>", $v;
    }
    echo "\n";
}
collect(x: 1);
collect(x: 1, y: 2);
collect(1, x: 2);
collect(1, 2, 3, x: 4, y: 5);
collect(x: 1, first: "named first");
collect(first: "f", x: 1);
collect(1, 2, first_: "almost", rest: "the variadic's own name");

function only(...$all) {
    $n = 0;
    foreach ($all as $k => $v) {
        echo $k, "=", $v, " ";
        $n++;
    }
    echo "(", $n, ")\n";
}
only(a: 1, b: 2);
only(1, 2, a: 3);
only(...[1, 2], ...["a" => 3], b: 4);
only(all: "named like the parameter");

function forward(...$args) {
    collect(...$args);
}
forward(1, 2, x: 3);
forward(x: 3, first: "via forward");
forward(first: "only first");

function modify(...$opts) {
    $opts["added"] = true;
    $opts["x"] = "changed";
    $n = 0;
    foreach ($opts as $v) {
        $n++;
    }
    return $n;
}
echo modify(x: 1, y: 2), " ", modify(), " ", modify(x: 1), "\n";

function order(...$named) {
    foreach ($named as $k => $v) {
        echo $k, ",";
    }
    echo "\n";
}
order(z: 1, a: 2, m: 3);
order(...["q" => 1, "b" => 2], a: 3);

function with_default($a, $b = "B", ...$more) {
    echo $a, $b;
    foreach ($more as $k => $v) {
        echo " ", $k, "=", $v;
    }
    echo "\n";
}
with_default(1, extra: "e");
with_default(a: 1, extra: "e");
with_default(extra: "e", a: 1, b: 2);
