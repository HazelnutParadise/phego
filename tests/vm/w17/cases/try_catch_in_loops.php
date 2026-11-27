<?php
// try_catch_in_loops: try inside loops with break and continue inside try and inside catch; break 2 from a nested loop inside try
class Skip extends Exception {}

function check($i)
{
    if ($i % 3 === 0) {
        throw new Skip("skip " . $i);
    }
    return $i;
}

$sum = 0;
for ($i = 1; $i <= 6; $i++) {
    try {
        if ($i === 5) {
            break;
        }
        $sum = $sum + check($i);
    } catch (Skip $e) {
        echo "caught: ", $e->getMessage(), "\n";
        continue;
    }
    echo "after try i=", $i, " sum=", $sum, "\n";
}
echo "loop 1 ended at i=", $i, " sum=", $sum, "\n";

$hits = 0;
foreach ([1, 2, 4, 3, 5] as $v) {
    try {
        check($v);
        $hits = $hits + 1;
    } catch (Skip $e) {
        echo "catch breaks at ", $v, "\n";
        break;
    }
}
echo "loop 2 hits: ", $hits, "\n";

$w = 0;
while ($w < 5) {
    $w++;
    try {
        if ($w % 2 === 0) {
            continue;
        }
        echo "odd w: ", $w, "\n";
    } catch (Skip $e) {
        echo "never\n";
    }
}
echo "while ended w=", $w, "\n";

$found = "none";
try {
    for ($a = 1; $a <= 3; $a++) {
        for ($b = 1; $b <= 3; $b++) {
            if ($a * $b === 4) {
                $found = $a . "*" . $b;
                break 2;
            }
        }
    }
    echo "after nested loops: ", $found, "\n";
} catch (Exception $e) {
    echo "never\n";
}
echo "found: ", $found, " a=", $a, " b=", $b, "\n";
