<?php
function show($label, $a) {
    echo $label, ":";
    foreach ($a as $k => $v) {
        echo " ", $k, "=", $v;
    }
    echo "\n";
}

function rest($first, ...$rest) {
    echo "first=", $first, " ";
    show("rest", $rest);
}
rest(1);
rest(1, 2);
rest(1, 2, 3, "four", 5.5);

function all(...$all) {
    $n = 0;
    foreach ($all as $v) {
        $n++;
    }
    show("all(" . $n . ")", $all);
}
all();
all("only");
all(null, false, true, 0);

function sum(...$nums) {
    $total = 0;
    foreach ($nums as $n) {
        $total += $n;
    }
    return $total;
}
echo sum(), " ", sum(1), " ", sum(1, 2, 3, 4), "\n";

function with_defaults($a, $b = "B", $c = "C", ...$more) {
    echo $a, $b, $c, " ";
    show("more", $more);
}
with_defaults(1);
with_defaults(1, 2);
with_defaults(1, 2, 3);
with_defaults(1, 2, 3, 4);
with_defaults(1, 2, 3, 4, 5);

function is_copy(...$items) {
    $items[0] = "changed inside";
    $items[] = "added";
    return $items[0];
}
$x = "original";
echo is_copy($x, 2), " ", $x, "\n";

function arrays_inside(...$groups) {
    foreach ($groups as $i => $g) {
        show("group " . $i, $g);
    }
}
arrays_inside([1, 2], ["k" => "v"], []);

function empty_is_array(...$none) {
    echo $none === [] ? "empty array" : "other", "\n";
    $none[] = 1;
    return $none;
}
show("grown", empty_is_array());
show("fresh", empty_is_array());
