<?php
function outer($a = 1, $b = "two", $c = [3])
{
    return middle($a);
}
function middle($x, $y = 10, $z = null)
{
    return inner($x, $y, "literal", 4.5, true, null, [1, 2]);
}
function inner($p = 1, $q = 2, $r = 3, $s = 4, $t = 5, $u = 6, $v = 7)
{
    return $p % ($q - $q);
}
echo outer(), "\n";
