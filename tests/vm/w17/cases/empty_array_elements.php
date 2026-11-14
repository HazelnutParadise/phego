<?php
$a = [0 => "", 1 => "0", 2 => "x", 3 => [], 4 => [0], 5 => null, 6 => false, 7 => 0.0, "k" => "0.0"];
foreach ([0, 1, 2, 3, 4, 5, 6, 7, 8, "k", "1", 1.0, true, null, "x"] as $k) {
    echo empty($a[$k]) ? "E" : "n";
}
echo "\n";
echo empty($a[4][0]) ? "e" : "n", empty($a[4][1]) ? "e" : "n", empty($a[3][0]) ? "e" : "n", empty($a[2][0]) ? "e" : "n", empty($a[2][1]) ? "e" : "n", "\n";
$r = &$a[2];
echo empty($a[2]) ? "e" : "n", "\n";
$r = "";
echo empty($a[2]) ? "e" : "n", "\n";
function check($arr, $key)
{
    return empty($arr[$key]) ? "e" : "n";
}
echo check($a, 0), check($a, 2), check($a, "nope"), check(null, 0), check("str", 0), check(5, 0), "\n";
$i = 1;
echo empty($a[$i++]) ? "e" : "n", $i, "\n";
