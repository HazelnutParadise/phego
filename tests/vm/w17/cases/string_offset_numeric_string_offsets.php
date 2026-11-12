<?php
$s = "hello";
echo $s["1"], "\n";
echo $s["-1"], "\n";
echo $s["0"], "\n";
echo $s["4"], "\n";
echo $s[" 1"], "\n";
echo $s["1 "], "\n";
$k = "2";
echo $s[$k], "\n";
$k = "-2";
echo $s[$k], "\n";
echo $s["5"], "|\n";
echo $s["+1"], "|\n";
