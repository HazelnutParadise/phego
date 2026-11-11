<?php
for ($i = 0; $i < 3; $i++) {
    switch ($i) {
        case 1:
            echo "one ";
            continue;
        default:
            echo "other", $i, " ";
    }
    echo "tail", $i, " ";
}
echo "\n";
