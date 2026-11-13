<?php
$a = ["a" => 1, "b" => 2];
foreach ($a as $x => $x) {
    echo $x, " ";
}
echo "| ", $x, "\n";
foreach (["p", "q"] as $i => $i) {
    echo $i, " ";
}
echo "| ", $i, "\n";
foreach ([10 => "x", 20 => "y"] as $k => $v) {
    echo $k, $v, " ";
}
echo "\n";
$same = [3 => 4, 5 => 6];
foreach ($same as $z => $z) {
    echo $z;
}
echo "\n";
