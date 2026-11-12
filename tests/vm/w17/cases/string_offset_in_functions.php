<?php
function first($s)
{
    return $s[0];
}
function last($s)
{
    return $s[-1];
}
function at($s, $i = 0)
{
    return $s[$i];
}
echo first("hello"), last("hello"), at("hello"), at("hello", 2), "\n";
echo first(""), "|\n";
echo at("ab", 5), "|\n";
echo first(123), "|\n";
echo first(null), "|\n";
function nth_digit($n, $pos)
{
    $s = $n . "";
    return $s[$pos] ?? "none";
}
echo nth_digit(9876, 0), nth_digit(9876, 3), nth_digit(9876, 4), nth_digit(9876, -1), "\n";
function chars($s)
{
    $out = "";
    for ($i = 0; isset($s[$i]); $i++) {
        $out .= $s[$i] . ".";
    }
    return $out;
}
echo chars("abc"), chars(""), chars("z"), "\n";
function warn_inside($s)
{
    $x = $s[9];
    return "returned";
}
echo warn_inside("short"), "\n";
