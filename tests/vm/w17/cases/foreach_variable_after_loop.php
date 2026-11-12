<?php
$a = ["x" => 1, "y" => 2, "z" => 3];
foreach ($a as $k => $v) {
}
echo $k, $v, "\n";
foreach ($a as $k => $v) {
    if ($v == 2) {
        break;
    }
}
echo $k, $v, "\n";
foreach ($a as $k => $v) {
    $k = "o";
    $v = "p";
}
echo $k, $v, "\n";
$u = 5;
foreach ($a as $u) {
}
echo $u, "\n";
foreach ($a as $k => $v) {
    continue;
}
echo $k, $v, "\n";
foreach ([[1, 2]] as $v) {
    $v[] = 3;
}
echo $v[0], $v[1], $v[2], "\n";
