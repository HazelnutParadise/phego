<?php
function level3($n, $s)
{
    return $n % 0;
}
function level2($n)
{
    return level3($n, "text arg");
}
function level1($a, $b)
{
    return level2($a + $b);
}
echo "calling\n";
echo level1(1, 2.5), "\n";
