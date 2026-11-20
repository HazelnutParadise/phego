<?php
function say($what, $value) {
    echo "<", $what, ">";
    return $value;
}
function abc($a = "-", $b = "-", $c = "-") {
    echo " => ", $a, $b, $c, "\n";
}
abc(c: say("c", 3), a: say("a", 1), b: say("b", 2));
abc(say("pos", 1), c: say("c", 3));
abc(b: say("outer", say("inner", 2)));

$i = 0;
abc(c: $i++, b: $i++, a: $i++);
abc(a: $i, b: $i = 10, c: $i);

$arr = [];
abc(b: $arr[] = "pushed", a: $arr[0] ?? "empty");

function side(&$r, $v = "d") {
    $r .= $v;
    return $r;
}
$acc = "";
abc(a: side(v: "1", r: $acc), b: side(r: $acc), c: $acc);

function defaults($x = "dx" . PHP_EOL, $y = "y") {
    echo $y, $x;
}
defaults(y: say("y", "Y"));

$u = "defined";
abc(a: $u, b: $u = "reassigned", c: $u);

function nested_named($n, $label = "L") {
    return $label . $n;
}
abc(
    c: nested_named(label: "c", n: 3),
    a: nested_named(n: 1),
    b: nested_named(2, label: "b")
);
