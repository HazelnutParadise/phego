<?php
$a = ["abc", "de", "k" => "xyz", 5 => ""];
echo $a[0][1], $a[1][-1], $a["k"][0], "\n";
echo $a[0][3], "|\n";
echo $a[2][0], "|\n";
echo $a["missing"][0], "|\n";
echo $a[5][0], "|\n";
echo $a["k"][1][0], "\n";
$m = [["ab", "cd"], ["ef"]];
echo $m[0][1][0], $m[1][0][-1], "\n";
echo $m[1][1][0], "|\n";
foreach ($a as $v) {
    echo $v[0] ?? "-", $v[1] ?? "-", " ";
}
echo "\n";
