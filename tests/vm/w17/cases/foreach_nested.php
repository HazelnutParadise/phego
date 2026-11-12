<?php
$grid = [[1, 2, 3], [4, 5, 6], [7, 8, 9]];
foreach ($grid as $r => $row) {
    foreach ($row as $c => $cell) {
        echo $r, $c, "=", $cell, " ";
    }
    echo "\n";
}
$sum = 0;
foreach ($grid as $row) {
    foreach ($row as $cell) {
        $sum += $cell;
    }
}
echo $sum, "\n";
foreach ([1, 2] as $a) {
    foreach ([10, 20] as $b) {
        foreach ([100, 200] as $c) {
            echo $a + $b + $c, " ";
        }
    }
}
echo "\n";
foreach ($grid as $row) {
    foreach ($row as $cell) {
    }
}
echo $row[0], $cell, "\n";
