<?php
$a = [3 => "c", 1 => "a", 2 => "b"];
foreach ($a as $k => $v) {
    echo $k, $v, " ";
}
echo "\n";
$a[0] = "z";
$a[] = "end";
unset($a[1]);
$a[1] = "back";
foreach ($a as $k => $v) {
    echo $k, $v, " ";
}
echo "\n";
$b = ["x" => 1];
$b["y"] = 2;
$b["x"] = 3;
unset($b["x"]);
$b["x"] = 4;
foreach ($b as $k => $v) {
    echo $k, $v, " ";
}
echo "\n";
