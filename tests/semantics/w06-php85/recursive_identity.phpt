--TEST--
Identity comparison of distinct recursive arrays throws a recursive dependency Error
--FILE--
<?php
$a = []; $a[0] = &$a; $b = []; $b[0] = &$b;
try { var_dump($a === $b); } catch (Error $e) { echo get_class($e), ": ", $e->getMessage(), "\n"; }
var_dump($a === $a);
$c = [1]; $d = [1]; var_dump($c === $d);
--EXPECT--
Error: Nesting level too deep - recursive dependency?
bool(true)
bool(true)
