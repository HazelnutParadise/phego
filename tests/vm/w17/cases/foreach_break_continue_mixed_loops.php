<?php
for ($i = 0; $i < 3; $i++) {
    foreach ([1, 2, 3] as $v) {
        if ($v == 2) {
            continue 2;
        }
        echo $i, $v, " ";
    }
}
echo "\n";
foreach ([1, 2, 3] as $v) {
    $n = 0;
    while (true) {
        $n++;
        if ($n > $v) {
            break;
        }
        if ($n == 2) {
            continue 2;
        }
        echo $v, $n, " ";
    }
    echo "end", $v, " ";
}
echo "\n";
foreach ([1, 2, 3] as $v) {
    switch ($v) {
        case 2:
            continue 2;
        default:
            echo "s", $v, " ";
            break;
    }
    echo "after", $v, " ";
}
echo "\n";
$n = 0;
do {
    foreach ([1, 2, 3] as $v) {
        if ($v == 3) {
            break 2;
        }
    }
    $n++;
} while ($n < 5);
echo $n, $v, "\n";
