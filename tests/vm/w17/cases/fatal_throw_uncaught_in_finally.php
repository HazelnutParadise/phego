<?php
// fatal_throw_uncaught_in_finally: finally throws while another exception is pending and nothing catches; the report shows the chain
function f()
{
    try {
        echo "try runs\n";
        throw new LogicException("first");
    } finally {
        echo "finally runs\n";
        throw new RuntimeException("second");
    }
}

function g()
{
    try {
        return "returned";
    } finally {
        echo "g finally, no throw\n";
    }
}

$gv = g();
echo "g: ", $gv, "\n";
echo "start\n";
f();
echo "never\n";
