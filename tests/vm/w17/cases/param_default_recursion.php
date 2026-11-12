<?php
function countdown($n, $step = 1, $acc = "")
{
    if ($n <= 0) {
        return $acc . "go";
    }
    return countdown($n - $step, $step, $acc . $n . ",");
}
echo countdown(5), "\n";
echo countdown(10, 3), "\n";
echo countdown(4, 2, "start:"), "\n";
function fact($n, $acc = 1)
{
    return $n <= 1 ? $acc : fact($n - 1, $acc * $n);
}
echo fact(5), " ", fact(10), " ", fact(1), "\n";
function fib($n, $a = 0, $b = 1)
{
    if ($n == 0) {
        return $a;
    }
    return fib($n - 1, $b, $a + $b);
}
echo fib(10), " ", fib(20), "\n";
function depth($n, $out = [])
{
    $out[] = $n;
    if ($n > 0) {
        return depth($n - 1, $out);
    }
    return $out;
}
$r = depth(3);
echo $r[0], $r[1], $r[2], $r[3], "\n";
$r2 = depth(1);
echo $r2[0], $r2[1], isset($r2[2]) ? "leak" : "clean", "\n";
