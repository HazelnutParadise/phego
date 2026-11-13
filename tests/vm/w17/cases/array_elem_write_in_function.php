<?php
function build($n)
{
    $r = [];
    for ($i = 0; $i < $n; $i++) {
        $r[$i] = $i * $i;
        $r["sq"][$i] = $i * $i;
        $r["sum"] += $i;
    }
    return $r;
}
$x = build(4);
echo $x[3], $x["sq"][2], $x["sum"], "\n";
function counter()
{
    $c["n"]++;
    $c["n"] += 10;
    $c["list"][] = $c["n"];
    return $c;
}
$y = counter();
echo $y["n"], $y["list"][0], "\n";
function undefined_local()
{
    $t[1][2] .= "x";
    return $t[1][2];
}
echo undefined_local(), "\n";
function inner_unset($a)
{
    unset($a[0][1], $a["z"]);
    return isset($a[0][1]) ? "kept" : "removed";
}
echo inner_unset([[1, 2]]), "\n";
