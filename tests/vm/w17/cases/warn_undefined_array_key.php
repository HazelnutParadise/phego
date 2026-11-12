<?php
$a = [1, "x" => 2];
echo $a[5], "|\n";
echo $a["nope"], "|\n";
echo $a["5"], "|\n";
echo $a[-1], "|\n";
$v = $a["x"] + $a["y"];
echo $v, "\n";
echo $a[1.5] ?? "coalesce", "\n";
$e = [];
echo $e[0], "|\n";
