<?php
$a = &$u;
echo "[", $u, "][", $a, "]\n";
$a = 4;
echo $u, "\n";
$x[] = 1;
$y = &$x[5];
echo "[", $y, "]\n";
$y = 9;
echo $x[5], "\n";
$z = &$n["k"]["j"];
$z = 7;
echo $n["k"]["j"], "\n";
