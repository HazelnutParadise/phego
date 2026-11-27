<?php
// fatal_throw_uncaught_from_function_finally: finally blocks at three call depths run before the uncaught exception is reported
function level_c()
{
    try {
        throw new RuntimeException("deep");
    } finally {
        echo "finally c\n";
    }
}

function level_b()
{
    try {
        level_c();
    } finally {
        echo "finally b\n";
    }
}

function level_a()
{
    try {
        level_b();
    } finally {
        echo "finally a\n";
    }
}

echo "start\n";
level_a();
echo "never\n";
