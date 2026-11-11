<?php
function inner($a, $b, $c)
{
    return $a;
}
function outer($x)
{
    return inner($x, "two");
}
echo "start\n";
outer(41);
