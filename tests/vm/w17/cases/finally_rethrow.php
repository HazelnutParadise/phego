<?php
// finally_rethrow: catch rethrows the same exception, finally runs, the outer catch gets it; try/finally without catch at three call depths
class Re extends Exception {}

function rethrower($obj)
{
    try {
        throw $obj;
    } catch (Re $e) {
        echo "rethrower catch\n";
        throw $e;
    } finally {
        echo "rethrower finally\n";
    }
}

$orig = new Re("same object");
try {
    rethrower($orig);
} catch (Re $e) {
    echo "outer got message: ", $e->getMessage(), "\n";
    echo "outer same object: ", $e === $orig ? "yes" : "no", "\n";
}

function depth3()
{
    try {
        throw new Re("deep");
    } finally {
        echo "depth3 finally\n";
    }
}

function depth2()
{
    try {
        depth3();
    } finally {
        echo "depth2 finally\n";
    }
}

function depth1()
{
    try {
        depth2();
    } finally {
        echo "depth1 finally\n";
    }
}

try {
    depth1();
} catch (Re $e) {
    echo "top caught: ", $e->getMessage(), "\n";
}
