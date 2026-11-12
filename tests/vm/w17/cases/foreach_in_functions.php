<?php
function total($arr)
{
    $t = 0;
    foreach ($arr as $v) {
        $t += $v;
    }
    return $t;
}
echo total([1, 2, 3, 4]), "\n";
echo total([]), "\n";
function show($arr)
{
    foreach ($arr as $k => $v) {
        echo $k, "=", $v, ",";
    }
    echo "\n";
    echo $k, $v, "\n";
}
show(["a" => 1, "b" => 2]);
function modify($arr)
{
    foreach ($arr as $k => $v) {
        $arr[$k] = $v . "!";
    }
    return $arr;
}
$r = modify(["x", "y"]);
echo $r[0], $r[1], "\n";
function nested_fn($grid)
{
    $out = "";
    foreach ($grid as $row) {
        foreach ($row as $c) {
            $out .= $c;
        }
        $out .= "/";
    }
    return $out;
}
echo nested_fn([[1, 2], [3], []]), "\n";
function recurse($arr, $depth)
{
    foreach ($arr as $v) {
        if ($depth > 0) {
            echo recurse($arr, $depth - 1);
        }
        echo $v, $depth, " ";
    }
    return "";
}
recurse([1, 2], 1);
echo "\n";
function leaks()
{
    foreach ([7, 8] as $leaked) {
    }
    return $leaked;
}
echo leaks(), isset($leaked) ? "global" : "local", "\n";
