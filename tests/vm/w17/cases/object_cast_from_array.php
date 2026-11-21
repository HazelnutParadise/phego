<?php
$o = (object)["a" => 1, "b" => [2, 3], "0" => "zero", 5 => "five"];
foreach ($o as $k => $v) {
    echo $k, " => ", ($k === "b" ? $v[0] . "," . $v[1] : $v), "\n";
}
echo "read: ", $o->a, " ", $o->b[1], "\n";
$o->c = "added";
$o->a++;
echo "after write: ", $o->a, " ", $o->c, "\n";
echo "numeric name via braces: ", $o->{"0"}, " ", $o->{"5"}, "\n";
echo "isset: ", (isset($o->a) ? "yes" : "no"), (isset($o->zz) ? "yes" : "no"), "\n";
unset($o->a);
echo "after unset: ", (isset($o->a) ? "yes" : "no"), "\n";

$back = (array)$o;
foreach ($back as $k => $v) {
    echo "back ", $k, "\n";
}
echo "back int key: ", $back[0], " ", $back[5], "\n";

$s = (object)"scalar";
echo "scalar wraps in scalar: ", $s->scalar, "\n";
$i = (object)42;
echo "int wraps: ", $i->scalar, "\n";
$e = (object)[];
$n = 0;
foreach ($e as $v) {
    $n++;
}
echo "empty object props: ", $n, "\n";
$nul = (object)null;
$n = 0;
foreach ($nul as $v) {
    $n++;
}
echo "null cast props: ", $n, "\n";

$same = (object)["x" => 1];
$again = (object)$same;
echo "object cast of object is identity: ", ($same === $again ? "yes" : "no"), "\n";
echo "equal casts: ", ((object)["x" => 1] == (object)["x" => 1] ? "yes" : "no"), "\n";
$o2 = (object)["k" => "v"];
$o2->k .= "w";
echo $o2->k, "\n";
