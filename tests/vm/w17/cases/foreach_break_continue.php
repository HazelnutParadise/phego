<?php
foreach ([1, 2, 3, 4, 5, 6] as $v) {
    if ($v == 2) {
        continue;
    }
    if ($v == 5) {
        break;
    }
    echo $v, " ";
}
echo "| ", $v, "\n";
foreach ([1, 2, 3] as $i) {
    foreach ([10, 20, 30] as $j) {
        if ($j == 20) {
            continue;
        }
        if ($i == 2) {
            break;
        }
        echo $i * $j, " ";
    }
}
echo "\n";
foreach ([1, 2, 3] as $i) {
    foreach ([10, 20, 30] as $j) {
        if ($j == 20) {
            continue 2;
        }
        echo $i + $j, " ";
    }
    echo "unreached ";
}
echo "\n";
foreach ([1, 2, 3] as $i) {
    foreach ([10, 20, 30] as $j) {
        foreach ([100, 200] as $k) {
            if ($i == 2) {
                break 3;
            }
            if ($k == 200) {
                continue 3;
            }
            echo $i + $j + $k, " ";
        }
    }
}
echo "| ", $i, $j, $k, "\n";
