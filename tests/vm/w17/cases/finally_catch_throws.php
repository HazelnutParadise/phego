<?php
// finally_catch_throws: a catch block throws; finally still runs; the outer catch receives the catch block's exception
class Orig extends Exception {}
class FromCatch extends Exception {}

function handler()
{
    try {
        throw new Orig("original");
    } catch (Orig $e) {
        echo "handler catch: ", $e->getMessage(), "\n";
        throw new FromCatch("thrown by catch");
    } finally {
        echo "handler finally\n";
    }
    echo "handler never\n";
}

try {
    handler();
} catch (FromCatch $e) {
    echo "outer got: ", $e::class, " ", $e->getMessage(), "\n";
    echo "original was chained: ", $e->getPrevious() === null ? "no" : "yes", "\n";
}

function wrapper()
{
    try {
        throw new Orig("inner");
    } catch (Orig $e) {
        throw new FromCatch("wrapped", 0, $e);
    } finally {
        echo "wrapper finally\n";
    }
}

try {
    wrapper();
} catch (FromCatch $e) {
    $p = $e->getPrevious();
    echo "wrapped previous class: ", $p::class, "\n";
    echo "wrapped previous message: ", $p->getMessage(), "\n";
}
