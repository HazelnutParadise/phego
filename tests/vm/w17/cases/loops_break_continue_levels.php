<?php
for ($i = 0; $i < 4; $i++) {
    for ($j = 0; $j < 4; $j++) {
        if ($j == 2) {
            continue 2;
        }
        if ($i == 3) {
            break 2;
        }
        echo $i, $j, " ";
    }
    echo "never ";
}
echo "\n";
$i = 0;
while (true) {
    $i++;
    $j = 0;
    while (true) {
        $j++;
        if ($j > 2) {
            break;
        }
        if ($i == 2) {
            continue 2;
        }
        echo $i, ":", $j, " ";
    }
    if ($i >= 3) {
        break 1;
    }
}
echo "\n";
$k = 0;
do {
    $k++;
    if ($k % 2) {
        continue;
    }
    echo "even", $k, " ";
} while ($k < 6);
echo "\n";
for ($a = 0; $a < 3; $a++) {
    switch ($a) {
        case 1:
            continue 2;
        default:
            echo "s", $a, " ";
    }
    echo "after", $a, " ";
}
echo "\n";
