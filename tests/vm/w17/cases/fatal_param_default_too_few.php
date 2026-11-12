<?php
function f($a, $b = 1, $c = 2)
{
    return $a + $b + $c;
}
echo f(1), "\n";
echo f(1, 2, 3), "\n";
echo "calling with none\n";
echo f(), "\n";
echo "not reached\n";
