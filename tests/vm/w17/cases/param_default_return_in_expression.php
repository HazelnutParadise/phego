<?php
function twice($n = 2)
{
    return $n * 2;
}
echo twice(), twice(twice()), twice(twice(twice())), "\n";
echo twice() + twice(10) * twice(), "\n";
function cmp($a = 1, $b = 2)
{
    return $a <=> $b;
}
echo cmp(), cmp(2), cmp(2, 2), cmp(3, 1), "\n";
function pick($first = "f", $second = "s")
{
    return $first ?: $second;
}
echo pick(), pick(0), pick(0, 0), pick("", "alt"), "\n";
function maybe($x = null)
{
    return $x ?? "fallback";
}
echo maybe(), maybe(0), maybe(null), maybe(false), "\n";
