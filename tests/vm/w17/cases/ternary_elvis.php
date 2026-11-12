<?php
$x = 5;
echo $x > 3 ? "big" : "small", "\n";
echo $x ?: "falsy", "\n";
$z = 0;
echo $z ?: "falsy", "\n";
echo ($x > 3 ? ($x > 4 ? "gt4" : "eq4") : "le3"), "\n";
echo ($x < 3 ? "a" : ($x < 6 ? "b" : "c")), "\n";
echo $z ?: $x ?: "none", "\n";
echo "0" ?: "string zero falsy", "\n";
echo "0.0" ?: "never", "\n";
echo [] ?: "empty array falsy", "\n";
$r = true ? "t" : "f";
echo $r, "\n";
