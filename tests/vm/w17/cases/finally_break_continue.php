<?php
// finally_break_continue: break and continue inside try within loops run finally first; break 2 through two nested try/finally; continue in catch with finally
class Hop extends Exception {}

for ($i = 1; $i <= 3; $i++) {
    try {
        if ($i === 2) {
            echo "break at ", $i, "\n";
            break;
        }
        echo "body ", $i, "\n";
    } finally {
        echo "finally ", $i, "\n";
    }
}
echo "after loop i=", $i, "\n";

for ($j = 1; $j <= 2; $j++) {
    try {
        if ($j === 1) {
            continue;
        }
        echo "j body ", $j, "\n";
    } finally {
        echo "j finally ", $j, "\n";
    }
}

for ($a = 1; $a <= 2; $a++) {
    try {
        for ($b = 1; $b <= 2; $b++) {
            try {
                if ($a === 1 && $b === 2) {
                    echo "break 2 now\n";
                    break 2;
                }
                echo "pair ", $a, $b, "\n";
            } finally {
                echo "inner finally ", $a, $b, "\n";
            }
        }
    } finally {
        echo "outer finally a=", $a, "\n";
    }
}
echo "nested done a=", $a, "\n";

for ($k = 1; $k <= 2; $k++) {
    try {
        throw new Hop("k" . $k);
    } catch (Hop $e) {
        echo "catch ", $e->getMessage(), "\n";
        continue;
    } finally {
        echo "finally k=", $k, "\n";
    }
    echo "never after try\n";
}
echo "done k=", $k, "\n";
