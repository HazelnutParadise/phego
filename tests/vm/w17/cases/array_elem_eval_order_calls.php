<?php
function f($v)
{
    echo "f", $v, " ";
    return $v;
}
function g($v)
{
    echo "g", $v, " ";
    return $v;
}
function h($v)
{
    echo "h", $v, " ";
    return $v;
}
$a = [];
$a[f(1)][g(2)] = h(3);
echo "\n";
echo $a[1][2], "\n";
$a[f(1)][g(2)] += h(10);
echo "\n";
echo $a[1][2], "\n";
$a[f(4)] = [];
echo "\n";
$a[f(4)][] = h(5);
echo "\n";
$a[f(4)][g(0)] .= h("s");
echo "\n";
$a[f(6)] ??= h(7);
echo "\n";
$a[f(6)] ??= h(8);
echo "\n";
$a[f(1)][g(2)]++;
echo "\n";
echo $a[1][2], "\n";
$a[h(0)][f(0)][g(0)] = h(h(1));
echo "\n";
