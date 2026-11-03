--TEST--
Reference aliases survive unsetting the original variable
--FILE--
<?php
$original = 1;
$alias =& $original;
$original = 2;
echo $alias, "\n";
unset($original);
echo $alias, "\n";
$alias = 3;
echo $alias, "\n";
var_dump(array_key_exists('original', get_defined_vars()));
?>
--EXPECT--
2
2
3
bool(false)
