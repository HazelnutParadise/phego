<?php
$a = [1, 2, 3];
$r = &$a[1];
unset($a[1]);
$r = 5;
echo isset($a[1]) ? "set" : "unset", " ", $r, "\n";
$a[1] = 6;
echo $r, "\n";
$p = 1;
$q = &$p;
$q = &$other;
$q = 2;
echo $p, " ", $other, "\n";
$m = [1];
$n = &$m;
$n[] = 2;
echo $m[1], "\n";
$m = "s";
echo $n, "\n";
