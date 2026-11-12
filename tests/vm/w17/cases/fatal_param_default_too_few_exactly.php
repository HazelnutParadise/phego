<?php
function f($a, $b)
{
    return $a . $b;
}
function g($a, $b = "b")
{
    return f($a);
}
echo g("x", "y"), "\n";
echo "not reached\n";
