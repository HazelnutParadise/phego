<?php
$a = [1, 2, 3];
foreach ($a as &$v) {
    $v *= 2;
}
echo $a[0], $a[1], $a[2], "\n";
$v = 100;
echo $a[2], "\n";
unset($v);
foreach ($a as $k => &$v) {
    $v = $k . ":" . $v;
}
echo $a[0], " ", $a[1], " ", $a[2], "\n";
foreach ($a as $v) {
}
echo $a[0], " ", $a[1], " ", $a[2], "\n";
