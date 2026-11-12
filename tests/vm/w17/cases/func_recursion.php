<?php
function fact($n)
{
    if ($n <= 1) {
        return 1;
    }
    return $n * fact($n - 1);
}
function fib($n)
{
    return $n < 2 ? $n : fib($n - 1) + fib($n - 2);
}
function depth($n)
{
    if ($n == 0) {
        return 0;
    }
    return 1 + depth($n - 1);
}
function isEven($n)
{
    return $n == 0 ? true : isOdd($n - 1);
}
function isOdd($n)
{
    return $n == 0 ? false : isEven($n - 1);
}
echo fact(10), "\n";
echo fact(20), "\n";
echo fact(21), "\n";
echo fib(20), "\n";
echo depth(500), "\n";
echo isEven(301) ? "even" : "odd", "\n";
