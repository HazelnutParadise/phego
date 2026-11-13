<?php
function f($a = 1 % 0, $b = 7)
{
    return $a . $b;
}
echo f(5), "\n";
echo f(6, 8), "\n";
echo "calling with default\n";
echo f(), "\n";
echo "not reached\n";
