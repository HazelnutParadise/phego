<?php
// closure_recursion: recursion through use (&$f) and mutual recursion through references
$fact = function ($n) use (&$fact) {
    if ($n <= 1) {
        return 1;
    }
    return $n * $fact($n - 1);
};
echo "fact_0: ", $fact(0), "\n";
echo "fact_5: ", $fact(5), "\n";
echo "fact_10: ", $fact(10), "\n";

$fib = function ($n) use (&$fib) {
    if ($n < 2) {
        return $n;
    }
    return $fib($n - 1) + $fib($n - 2);
};
echo "fib_0: ", $fib(0), "\n";
echo "fib_1: ", $fib(1), "\n";
echo "fib_10: ", $fib(10), "\n";

$isEven = function ($n) use (&$isEven, &$isOdd) {
    if ($n === 0) {
        return "even";
    }
    return $isOdd($n - 1);
};
$isOdd = function ($n) use (&$isEven, &$isOdd) {
    if ($n === 0) {
        return "odd";
    }
    return $isEven($n - 1);
};
echo "mutual_4: ", $isEven(4), "\n";
echo "mutual_7: ", $isEven(7), "\n";

$depth = 0;
$walk = function ($n) use (&$walk, &$depth) {
    $depth = $depth + 1;
    if ($n > 0) {
        $walk($n - 1);
    }
};
$walk(4);
echo "depth_count: ", $depth, "\n";

$collect = function ($n, &$acc) use (&$collect) {
    if ($n === 0) {
        return;
    }
    $acc[] = $n;
    $collect($n - 1, $acc);
};
$list = [];
$collect(3, $list);
foreach ($list as $idx => $v) {
    echo "acc[$idx]=$v\n";
}
