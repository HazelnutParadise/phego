<?php
$a = [1, 2, 3];
foreach ($a as $k => &$v) {
    echo $k, "=", $v, " ";
    if ($k == 0) {
        $a[] = 4;
    }
    if ($k == 1) {
        unset($a[2]);
    }
}
echo "\n";
$b = ["x" => 1, "y" => 2];
foreach ($b as $k => &$v) {
    echo $k, " ";
    if ($k == "x") {
        $b["z"] = 3;
    }
}
echo "\n";
$c = [1, 2, 3];
foreach ($c as &$v) {
    echo $v;
    unset($c[1]);
}
echo "\n";
