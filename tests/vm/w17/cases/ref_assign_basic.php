<?php
$a = 1;
$b = &$a;
$b = 2;
echo $a, " ", $b, "\n";
$a = "x";
echo $a, " ", $b, "\n";
unset($b);
$b = 3;
echo $a, " ", $b, "\n";
$c = &$a;
$d = &$c;
$d .= "y";
echo $a, $c, $d, "\n";
$e = 5;
$c = &$e;
$c++;
echo $a, " ", $e, "\n";
