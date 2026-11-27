<?php
// finally_loop_inside: loops with break and continue fully inside a finally block; try/finally inside finally
function countUp()
{
    try {
        echo "countUp try\n";
    } finally {
        $total = 0;
        for ($i = 1; $i <= 10; $i++) {
            if ($i % 2 === 0) {
                continue;
            }
            if ($i > 6) {
                break;
            }
            $total = $total + $i;
        }
        echo "countUp total: ", $total, " stopped at i=", $i, "\n";
    }
    return "countUp done";
}
$r = countUp();
echo $r, "\n";

function finallyWithNested()
{
    try {
        echo "nested try\n";
    } finally {
        try {
            echo "inner try in finally\n";
            $n = 0;
            while (true) {
                $n++;
                if ($n === 3) {
                    break;
                }
            }
            echo "inner while n=", $n, "\n";
        } finally {
            echo "inner finally in finally\n";
        }
        echo "outer finally tail\n";
    }
    return "finallyWithNested done";
}
$r = finallyWithNested();
echo $r, "\n";

function foreachInFinally()
{
    $words = ['keep', 'skip', 'keep2'];
    try {
        return 0;
    } finally {
        foreach ($words as $w) {
            if ($w === 'skip') {
                continue;
            }
            echo "word: ", $w, "\n";
        }
    }
}
$r = foreachInFinally();
echo "foreach result: ", $r, "\n";
