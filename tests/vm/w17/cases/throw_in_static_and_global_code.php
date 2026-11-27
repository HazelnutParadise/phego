<?php
// throw_in_static_and_global_code: state changed before a throw from a static variable, a global and top-level code is kept
$total = 0;

function counter() {
    static $n = 0;
    $n = $n + 1;
    if ($n == 2) {
        throw new Exception("n is 2");
    }
    return $n;
}

function add_global($amount) {
    global $total;
    $total = $total + $amount;
    if ($total > 15) {
        throw new Exception("total too big: " . $total);
    }
    return $total;
}

echo "first: ", counter(), "\n";
try {
    counter();
    echo "not reached\n";
} catch (Exception $e) {
    echo "second_threw: ", $e->getMessage(), "\n";
}
echo "third: ", counter(), "\n";

echo "add 10: ", add_global(10), "\n";
try {
    add_global(10);
} catch (Exception $e2) {
    echo "global_threw: ", $e2->getMessage(), "\n";
}
echo "total_after: ", $total, "\n";

$g = 5;
try {
    $g = 6;
    throw new Exception("top level");
} catch (Exception $e3) {
    echo "top_level_caught: ", $e3->getMessage(), "\n";
}
echo "g_kept: ", $g, "\n";
echo "fourth: ", counter(), "\n";
echo "end\n";
