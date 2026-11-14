<?php
function f($a, $b)
{
    echo $u;
    return $a + $b;
}
echo @f(1, 2), "\n";
echo @f("x", [1]);
