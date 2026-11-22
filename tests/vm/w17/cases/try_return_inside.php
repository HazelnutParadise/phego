<?php
// try_return_inside: return from inside try and from inside catch in functions (no finally); values returned
class Miss extends Exception {}

function fromTry($x)
{
    try {
        if ($x > 0) {
            return "try:" . $x;
        }
        echo "fromTry: fallthrough\n";
    } catch (Miss $e) {
        return "never";
    }
    return "after try";
}

function fromCatch($x)
{
    try {
        if ($x > 0) {
            throw new Miss("m" . $x);
        }
        echo "fromCatch: no throw, no return\n";
    } catch (Miss $e) {
        return "catch:" . $e->getMessage();
    }
    return "after try";
}

function nestedReturn($x)
{
    try {
        try {
            if ($x === 1) {
                return "inner";
            }
        } catch (Miss $e) {
            return "inner catch";
        }
        return "outer";
    } catch (Exception $e) {
        return "outer catch";
    }
}

function passThrough($x)
{
    try {
        return fromCatch($x);
    } catch (Miss $e) {
        return "never";
    }
}

$r = fromTry(5);
echo "a: ", $r, "\n";
$r = fromTry(0);
echo "b: ", $r, "\n";
$r = fromCatch(0);
echo "c: ", $r, "\n";
$r = fromCatch(2);
echo "d: ", $r, "\n";
$r = nestedReturn(1);
echo "e: ", $r, "\n";
$r = nestedReturn(2);
echo "f: ", $r, "\n";
$r = passThrough(3);
echo "g: ", $r, "\n";
