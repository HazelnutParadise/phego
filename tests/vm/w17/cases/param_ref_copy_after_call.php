<?php
function bump(&$x) { $x = $x + 1; }
$a = [1, 2];
bump($a[0]);
$b = $a;
$b[0] = 100;
echo $a[0], " ", $b[0], "\n";
$r = &$a[1];
bump($a[1]);
$c = $a;
$c[1] = 200;
echo $a[1], " ", $c[1], "\n";
