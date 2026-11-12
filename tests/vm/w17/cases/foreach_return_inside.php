<?php
function find($arr, $needle)
{
    foreach ($arr as $k => $v) {
        if ($v == $needle) {
            return $k;
        }
    }
    return "none";
}
echo find([5, 6, 7], 7), "\n";
echo find(["a" => 1, "b" => 2], 2), "\n";
echo find([1], 9), "\n";
function first_pair($grid)
{
    foreach ($grid as $r => $row) {
        foreach ($row as $c => $cell) {
            if ($cell > 4) {
                return $r . "," . $c;
            }
        }
    }
    return "no";
}
echo first_pair([[1, 2], [3, 5], [9]]), "\n";
echo first_pair([[1]]), "\n";
function early()
{
    foreach ([1, 2, 3] as $v) {
        echo "iter", $v, " ";
        if ($v == 2) {
            return "returned at " . $v;
        }
    }
    echo "unreached";
}
echo early(), "\n";
