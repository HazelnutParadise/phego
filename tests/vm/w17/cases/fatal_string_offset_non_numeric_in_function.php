<?php
function pick($s, $k)
{
    return $s[$k];
}
function wrap($s, $k = "name")
{
    return pick($s, $k);
}
echo wrap("hello", 1), "\n";
echo wrap("hello"), "\n";
echo "not reached\n";
