<?php
$a = [1, 2, 3];
foreach ($a as $v) {
    $v = $v * 100;
    echo $v, " ";
}
echo "\n";
echo $a[0], $a[1], $a[2], "\n";
foreach ($a as $k => $v) {
    $k = "changed";
    $v .= "x";
    echo $k, $v, " ";
}
echo "\n";
foreach ($a as $v) {
    $a2[] = $v;
    $v++;
}
echo $a2[0], $a2[2], $v, "\n";
foreach ([5, 6] as $v) {
    $v += 1;
    $v .= "!";
    echo $v, " ";
}
echo "\n";
$v = "outside";
foreach ([1] as $v) {
}
echo $v, "\n";
foreach ([1, 2] as $k => $v) {
    $k = 10;
    echo $k, " ";
}
echo "\n";
