<?php
class A { public int $i = PHP_INT_MIN; }
$a = new A;
$a->i--;
echo $a->i, "|", $a->i === PHP_INT_MIN ? "min" : "changed", "\n";
$a->i -= 1;
