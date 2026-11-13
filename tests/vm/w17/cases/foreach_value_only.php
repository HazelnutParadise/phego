<?php
$a = [10, 20, 30];
foreach ($a as $v) {
    echo $v, "\n";
}
echo "after: ", $v, "\n";
$b = ["x" => "a", "y" => "b"];
foreach ($b as $v) {
    echo $v;
}
echo "\n";
$mixed = [1, "two", 3.5, true, null, false];
foreach ($mixed as $v) {
    echo "[", $v, "]";
}
echo "\n";
