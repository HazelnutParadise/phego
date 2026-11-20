<?php
function swap(&$left, &$right) {
    $t = $left;
    $left = $right;
    $right = $t;
}
$a = "A";
$b = "B";
swap(right: $b, left: $a);
echo $a, $b, "\n";
swap(left: $a, right: $b);
echo $a, $b, "\n";

function fill($value, &$target, $suffix = "") {
    $target = $value . $suffix;
    $value = "not visible";
}
$v = "v";
fill(target: $out, value: $v);
echo $out, " ", $v, "\n";
fill(suffix: "!", target: $made["k"][], value: "pushed");
echo $made["k"][0], "\n";

$list = [1, 2, 3];
fill(value: "second", target: $list[1]);
echo $list[0], $list[1], $list[2], "\n";

function refs(&...$r) {
    foreach ($r as $k => &$x) {
        $x = $k;
    }
}
refs(one: $n1, two: $n2);
echo $n1, " ", $n2, "\n";
$p0 = 0;
refs($p0, named: $n3);
echo $p0, " ", $n3, "\n";

function mixed($x, &...$r) {
    foreach ($r as $k => $v) {
        $r[$k] = $x . $k;
    }
}
mixed("m", a: $ma, b: $mb);
echo $ma, " ", $mb, "\n";

function default_ref($plain = "p", &$ref = "default") {
    $ref = $plain . "-" . $ref;
}
$r = "r";
default_ref(ref: $r);
echo $r, "\n";
default_ref(plain: "only plain");
echo $r, "\n";

function same(&$x, &$y) {
    $x = "x";
    echo $y, "\n";
    $y = "y";
}
$s = "s";
same(y: $s, x: $s);
echo $s, "\n";

$copy_source = [1, 2];
$copy = $copy_source;
fill(target: $copy_source[0], value: "written");
echo $copy_source[0], " ", $copy[0], "\n";
