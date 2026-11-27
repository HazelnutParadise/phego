<?php
// fatal_throw_uncaught_previous: uncaught exception with a two-level previous chain created in different functions
function inner()
{
    throw new InvalidArgumentException("inner problem", 1);
}

function middle()
{
    try {
        inner();
    } catch (Exception $e) {
        throw new RuntimeException("middle problem", 2, $e);
    }
}

echo "start\n";
middle();
echo "never\n";
