--TEST--
Foreach by reference stops with a warning when the variable stops being an array
--FILE--
<?php
$a = [1, 2]; $seen = [];
foreach ($a as &$v) { $seen[] = $v; $a = null; }
unset($v); echo implode(',', $seen), "\n"; var_dump($a);
$b = [1, 2]; $seen = [];
foreach ($b as &$v) { $seen[] = $v; $b = 5; }
unset($v); echo implode(',', $seen), "\n"; var_dump($b);
--EXPECTF--

Warning: foreach() argument must be of type array|object, null given in %s on line 3
1
NULL

Warning: foreach() argument must be of type array|object, int given in %s on line 6
1
int(5)
