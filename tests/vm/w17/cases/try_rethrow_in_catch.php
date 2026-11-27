<?php
// try_rethrow_in_catch: catch one exception and throw a new one with the first as previous; throw without previous; rethrow from a nested function
class LowLevel extends RuntimeException {}
class HighLevel extends LogicException {}

function openSource($n)
{
    throw new LowLevel("read failed #" . $n);
}

function loadConfig($n)
{
    try {
        openSource($n);
    } catch (LowLevel $e) {
        throw new HighLevel("config broken", 7, $e);
    }
}

function withoutPrev()
{
    try {
        throw new LowLevel("a");
    } catch (LowLevel $e) {
        throw new HighLevel("plain");
    }
}

function rethrowHelper($e)
{
    throw $e;
}

function nestedRethrow()
{
    try {
        throw new LowLevel("nested");
    } catch (LowLevel $e) {
        rethrowHelper($e);
    }
}

try {
    loadConfig(1);
} catch (HighLevel $e) {
    echo "outer class: ", $e::class, "\n";
    echo "outer message: ", $e->getMessage(), "\n";
    echo "outer code: ", $e->getCode(), "\n";
    $p = $e->getPrevious();
    echo "previous class: ", $p::class, "\n";
    echo "previous message: ", $p->getMessage(), "\n";
    echo "previous has previous: ", $p->getPrevious() === null ? "no" : "yes", "\n";
}

try {
    withoutPrev();
} catch (HighLevel $e) {
    echo "no previous: ", $e->getPrevious() === null ? "yes" : "no", "\n";
}

try {
    nestedRethrow();
} catch (LowLevel $e) {
    echo "rethrown from helper: ", $e->getMessage(), "\n";
}
