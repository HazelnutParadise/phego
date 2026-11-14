<?php
echo empty(0), empty("0"), empty("0.0"), empty([]), empty(0.0), empty(null), empty(false), "|", empty(" "), empty([0]), empty("a"), empty(1), empty(-1), empty(0.1), empty(true), "|\n";
$vals = [0, "0", "", "0.0", [], 0.0, -0.0, null, false, " ", [0], "a", 1, -1, 0.1, true, "00", [null]];
foreach ($vals as $v) {
    echo empty($v) ? "E" : "n";
}
echo "\n";
$zero = 0;
$str = "0";
$nul = null;
echo empty($zero) ? "e" : "n", empty($str) ? "e" : "n", empty($nul) ? "e" : "n", "\n";
$x = empty($zero);
echo $x === true ? "bool" : "other", "\n";
echo empty($zero) + empty($str) + empty("x"), "\n";
