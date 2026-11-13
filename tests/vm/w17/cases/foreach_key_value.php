<?php
$a = ["a" => 1, "b" => 2, 7 => 3, "c" => 4];
foreach ($a as $k => $v) {
    echo $k, "=", $v, "\n";
}
echo "after: ", $k, $v, "\n";
foreach ([5 => "x", "5" => "y", "05" => "z", -1 => "n"] as $k => $v) {
    echo $k, ":", $v, " ";
}
echo "\n";
foreach ([1, 2, 3] as $i => $v) {
    echo $i * $v, " ";
}
echo "\n";
