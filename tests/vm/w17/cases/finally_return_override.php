<?php
// finally_return_override: return in finally replaces the try's return value; return in finally swallows a pending exception; return in finally wins over the catch's return
class Pending extends Exception {}

function replaceTry()
{
    try {
        return "from try";
    } finally {
        echo "replaceTry finally\n";
        return "from finally";
    }
}
$r = replaceTry();
echo "1: ", $r, "\n";

function swallow()
{
    try {
        throw new Pending("lost");
    } finally {
        echo "swallow finally\n";
        return "exception swallowed";
    }
}
$r = swallow();
echo "2: ", $r, "\n";

function replaceCatch()
{
    try {
        throw new Pending("caught then replaced");
    } catch (Pending $e) {
        return "from catch: " . $e->getMessage();
    } finally {
        return "from finally wins";
    }
}
$r = replaceCatch();
echo "3: ", $r, "\n";

function eatRethrow()
{
    try {
        try {
            throw new Pending("inner");
        } catch (Pending $e) {
            throw new Pending("rethrown");
        } finally {
            return "finally eats the rethrow";
        }
    } catch (Pending $e) {
        return "never: " . $e->getMessage();
    }
}
$r = eatRethrow();
echo "4: ", $r, "\n";

function doubleReturn()
{
    try {
        return "first";
    } finally {
        return "second";
    }
}
$r = doubleReturn();
echo "5: ", $r, "\n";
echo "end\n";
