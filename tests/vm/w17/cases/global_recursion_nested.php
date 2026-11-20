<?php
$count = 0;
$log = [];

function down($n) {
    global $count;
    $count++;
    if ($n > 0) {
        down($n - 1);
    }
    note("down " . $n);
}

function note($text) {
    global $log, $count;
    $log[] = $text . " at " . $count;
}

function outer() {
    global $count;
    $count = 100;
    inner();
    echo "outer sees ", $count, "\n";
}

function inner() {
    global $count;
    $count += 5;
    innermost();
}

function innermost() {
    $count = "local to innermost";
    global $count;
    $count *= 2;
}

down(3);
echo $count, "\n";
foreach ($log as $line) {
    echo $line, "\n";
}
outer();
echo $count, "\n";

function fib_calls($n) {
    global $calls;
    $calls++;
    if ($n < 2) {
        return $n;
    }
    return fib_calls($n - 1) + fib_calls($n - 2);
}
echo fib_calls(10), " in ", $calls, " calls\n";
