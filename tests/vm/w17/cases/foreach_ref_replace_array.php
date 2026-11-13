<?php
$a = [1, 2, 3];
foreach ($a as $k => &$v) {
    echo $v, " ";
    if ($v == 1) {
        $a = [7, 8, 9, 10];
    }
}
echo "\n";
$b = [1, 2];
foreach ($b as &$v) {
    echo $v, " ";
    $b = "str";
}
echo "\n", $b, "\n";
$c = [1, 2, 3];
$copy = $c;
foreach ($c as &$v) {
    $v = 0;
}
echo $copy[0], $copy[1], $copy[2], $c[0], "\n";
