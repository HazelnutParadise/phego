<?php
function takes_int(int $n)
{
    return $n;
}
function returns_int($v): int
{
    return $v;
}
class Holder
{
    public int $n = 0;
}

echo "integral float: ", takes_int(2.0), "\n";
echo "fractional float: ", takes_int(2.5), "\n";
echo "fractional numeric string: ", takes_int("3.7"), "\n";
echo "negative fractional: ", takes_int(-1.9), "\n";
echo "return: ", returns_int(4.2), "\n";
$h = new Holder;
$h->n = 5.5;
echo "property: ", $h->n, "\n";
echo "integral numeric string with exponent: ", takes_int("2e1"), "\n";
