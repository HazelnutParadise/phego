<?php
// finally_after_caught: an exception caught by catch, then finally runs; an outer try catches after an inner try/finally without catch
class Boom extends Exception {}

function caughtHere()
{
    try {
        echo "A try\n";
        throw new Boom("A");
    } catch (Boom $e) {
        echo "A catch: ", $e->getMessage(), "\n";
    } finally {
        echo "A finally\n";
    }
    echo "A after\n";
}
caughtHere();

try {
    try {
        echo "B try\n";
        throw new Boom("B");
    } finally {
        echo "B inner finally\n";
    }
    echo "B not reached\n";
} catch (Boom $e) {
    echo "B outer catch: ", $e->getMessage(), "\n";
} finally {
    echo "B outer finally\n";
}
echo "end\n";
