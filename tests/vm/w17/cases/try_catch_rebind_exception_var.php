<?php
// try_catch_rebind_exception_var: the same variable name in several catch clauses in sequence and nested; the inner catch overwrites the outer $e
class Tag1 extends Exception {}
class Tag2 extends Exception {}

function t1()
{
    throw new Tag1("one");
}

function t2()
{
    throw new Tag2("two");
}

try {
    t1();
} catch (Tag1 $e) {
    echo "seq 1: ", $e->getMessage(), "\n";
}

try {
    t2();
} catch (Tag2 $e) {
    echo "seq 2: ", $e->getMessage(), "\n";
}
echo "after seq e class: ", $e::class, "\n";

try {
    try {
        throw new Tag1("outer");
    } catch (Tag1 $e) {
        echo "outer catch e: ", $e->getMessage(), "\n";
        try {
            throw new Tag2("inner");
        } catch (Tag2 $e) {
            echo "inner catch e: ", $e->getMessage(), "\n";
        }
        echo "back in outer catch e: ", $e->getMessage(), "\n";
    }
} catch (Exception $e) {
    echo "wrong: outer try caught\n";
}
echo "final e class: ", $e::class, "\n";
