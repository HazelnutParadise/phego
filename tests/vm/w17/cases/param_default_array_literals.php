<?php
function f($a = [], $b = [1, 2, 3], $c = ["k" => "v", 5 => "five"], $d = [[1], [2, [3]]])
{
    echo count_items($a), "|", $b[0], $b[2], "|", $c["k"], $c[5], "|", $d[1][1][0], "\n";
}
function count_items($arr)
{
    $n = 0;
    foreach ($arr as $_) {
        $n++;
    }
    return $n;
}
f();
f([9, 9]);
f([], [4, 5, 6]);
function mutate($list = [1, 2])
{
    $list[] = 3;
    $list[0] = "changed";
    return count_items($list) . $list[0];
}
echo mutate(), "\n";
echo mutate(), "\n";
echo mutate([7]), "\n";
function nested($m = ["a" => ["b" => "deep"]])
{
    $m["a"]["c"] = "added";
    return $m["a"]["b"] . $m["a"]["c"];
}
echo nested(), nested(), "\n";
function empty_default($x = [])
{
    foreach ($x as $v) {
        echo "never";
    }
    $x["new"] = 1;
    return $x["new"];
}
echo empty_default(), empty_default(), "\n";
