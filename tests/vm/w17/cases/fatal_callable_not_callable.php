<?php
// An int is not a callable value. Creating a first-class callable from it is an
// fatal error that PHP raises at creation time.

function add_one($n)
{
    return $n + 1;
}

$good = add_one(...);
echo "function callable: ", $good(1), "\n";

$name = "add_one";
$viaName = $name(...);
echo "string name callable: ", $viaName(2), "\n";

$x = 5;
echo "before calling an int as a function\n";

$bad = $x(...);

echo "after creation, not reached\n";
