<?php
function inc(&$x)
{
    $x++;
}
$a = 1;
inc($a);
inc($a);
echo $a, "\n";
inc($u);
echo $u, "\n";
$arr = [1, 2];
inc($arr[1]);
inc($arr[5]);
echo $arr[1], " ", $arr[5], "\n";
inc($n["a"]["b"]);
echo $n["a"]["b"], "\n";
function set(&$x, $v)
{
    $x = $v;
}
set($m, [1]);
set($m[], 2);
echo $m[0], $m[1], "\n";
