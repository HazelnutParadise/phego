<?php
echo early(2, 3), "\n";
function early($a, $b)
{
    return $a * $b;
}
function noReturn($x)
{
    $x++;
}
function emptyReturn()
{
    return;
}
echo noReturn(1) === null ? "null" : "other", "\n";
echo emptyReturn() === null ? "null" : "other", "\n";
echo extra(1, 2, 3, 4), "\n";
function extra($a, $b)
{
    return $a + $b;
}
function modify($v)
{
    $v = $v . "!";
    return $v;
}
$orig = "hi";
echo modify($orig), " ", $orig, "\n";
function arr($a)
{
    $a[] = 9;
    return $a;
}
$x = [1];
$y = arr($x);
echo isset($x[1]) ? "changed" : "unchanged", " ", $y[1], "\n";
function CaseName()
{
    return "case-insensitive";
}
echo casename(), " ", CASENAME(), "\n";
