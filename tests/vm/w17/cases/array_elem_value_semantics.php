<?php
$a = [1, [2, 3], "k" => [4]];
$b = $a;
$b[0] = 10;
$b[1][0] = 20;
$b["k"][] = 5;
echo $a[0], $a[1][0], $a["k"][0], isset($a["k"][1]) ? "leak" : "isolated", "\n";
echo $b[0], $b[1][0], $b["k"][1], "\n";
function change($arr)
{
    $arr[0] = "changed";
    $arr[1][1] = "deep";
    $arr[] = "extra";
    return $arr;
}
$c = change($a);
echo $a[0], $a[1][1], isset($a[2]) ? "leak" : "isolated", "\n";
echo $c[0], $c[1][1], $c[2], "\n";
$d = [[1], [2]];
$e = $d;
$e[0][] = 99;
$d[1][] = 77;
echo $d[0][1] ?? "none", $d[1][1], $e[0][1], $e[1][1] ?? "none", "\n";
$x = $y = [1, 2];
$x[0]++;
echo $x[0], $y[0], "\n";
$f = [$d, $d];
$f[0][0][0] = "f";
echo $f[0][0][0], $f[1][0][0], $d[0][0], "\n";
