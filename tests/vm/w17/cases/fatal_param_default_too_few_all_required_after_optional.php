<?php
function f($a = 1, $b)
{
    return $a + $b;
}
echo f(1, 2), "\n";
echo "calling with one\n";
echo f(5), "\n";
echo "not reached\n";
