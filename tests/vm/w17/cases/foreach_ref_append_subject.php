<?php
$a = [];
foreach ($a[] as &$v) {
    echo "no";
}
foreach ($a as $k => $x) {
    echo $k, "\n";
}
$b = [];
foreach ($b[0][] as &$v) {
}
foreach ($b[1][]["k"] as &$v) {
}
echo isset($b[0][0]) ? "set" : "null", " ", isset($b[1][0]["k"]) ? "set" : "null", "\n";
$c = [[1]];
foreach ($c[0] as &$v) {
    $v = 2;
}
unset($v);
foreach ($c[] as $k => &$v) {
}
echo $c[0][0], isset($c[1]) ? "set" : "null", "\n";
