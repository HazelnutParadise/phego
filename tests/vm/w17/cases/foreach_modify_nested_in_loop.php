<?php
function count_marker($arr)
{
    $n = 0;
    foreach ($arr as $_) {
        $n++;
    }
    return $n;
}
$a = [[1, 2], [3, 4]];
foreach ($a as $k => $row) {
    $a[$k][] = 0;
    $row[] = 9;
    foreach ($row as $cell) {
        echo $cell, " ";
    }
    echo "\n";
}
foreach ($a as $row) {
    foreach ($row as $cell) {
        echo $cell, " ";
    }
    echo "\n";
}
$b = [1, 2, 3];
foreach ($b as $x) {
    foreach ($b as $y) {
        $b[] = $x . $y;
    }
    echo count_marker($b), " ";
}
echo "\n";
