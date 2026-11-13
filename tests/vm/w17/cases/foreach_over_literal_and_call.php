<?php
function make($n)
{
    echo "make", $n, " ";
    return [$n, $n * 2];
}
foreach ([1, 2, 3] as $v) {
    echo $v;
}
echo "\n";
foreach (make(3) as $v) {
    echo $v, " ";
}
echo "\n";
foreach (make(1) as $k => $v) {
    foreach (make(2) as $v2) {
        echo $k, $v, $v2, " ";
    }
}
echo "\n";
foreach (["a" => [1, 2], "b" => [3]] as $k => $v) {
    foreach ($v as $x) {
        echo $k, $x, " ";
    }
}
echo "\n";
function none()
{
}
foreach (none() as $v) {
    echo "unreachable";
}
echo "after none\n";
foreach ([[1, 2], [3, 4]][1] as $v) {
    echo $v, " ";
}
echo "\n";
