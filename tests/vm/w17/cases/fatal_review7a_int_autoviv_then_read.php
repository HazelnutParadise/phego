<?php
class A { public ?int $i = null; }
$x = new A;
$x->i[] = 1;
echo "after\n";
echo $x->i === null ? "null" : "notnull", "\n";
foreach ((array)$x as $k => $v) { echo $k, "=", (string)(is_arr($v)), "\n"; }
function is_arr($v) { return $v === [1] ? "array[1]" : "other"; }
