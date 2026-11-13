<?php
$s = "abc";
echo $s[3], "|\n";
echo $s[100], "|\n";
echo $s[-4], "|\n";
echo $s[-100], "|\n";
$e = "";
echo $e[0], "|\n";
echo $e[-1], "|\n";
$i = 7;
echo $s[$i], "|\n";
echo $s[$i - 5], "|\n";
$r = $s[10];
echo $r === null ? "null" : "other", "|\n";
echo $s[2] . $s[3] . $s[0], "|\n";
