<?php
function before()
{
    return MARK;
}
const MARK = "m";
echo before(), "\n";
function depth($n)
{
    return $n <= 0 ? BASE : depth($n - 1) . STEP;
}
const BASE = "b", STEP = "s";
echo depth(3), "\n";
function uses_predefined()
{
    return PHP_INT_SIZE . DIRECTORY_SEPARATOR . E_NOTICE;
}
echo uses_predefined(), "\n";
function shadow()
{
    $MARK = "variable";
    return $MARK . MARK;
}
echo shadow(), "\n";
const AFTER = "after";
echo AFTER, "\n";
