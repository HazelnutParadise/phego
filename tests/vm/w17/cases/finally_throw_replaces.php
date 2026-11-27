<?php
// finally_throw_replaces: a throw in finally while an exception is pending replaces it and chains the pending one as previous
class Zero extends Exception {}
class First extends Exception {}
class Second extends Exception {}
class Third extends Exception {}

function pendingThenFinallyThrows()
{
    try {
        throw new First("first pending", 0, new Zero("zero root"));
    } finally {
        echo "finally throws second\n";
        throw new Second("second from finally");
    }
}

try {
    pendingThenFinallyThrows();
} catch (Second $e) {
    echo "caught: ", $e::class, " ", $e->getMessage(), "\n";
    $cur = $e;
    while ($cur !== null) {
        echo "chain: ", $cur::class, " ", $cur->getMessage(), "\n";
        $cur = $cur->getPrevious();
    }
}

function normalThenFinallyThrows()
{
    try {
        echo "normal try\n";
    } finally {
        throw new Third("from finally only");
    }
}

try {
    normalThenFinallyThrows();
} catch (Third $e) {
    echo "third message: ", $e->getMessage(), "\n";
    echo "third has previous: ", $e->getPrevious() === null ? "no" : "yes", "\n";
}
