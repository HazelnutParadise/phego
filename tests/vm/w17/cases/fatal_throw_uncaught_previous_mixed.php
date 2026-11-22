<?php
// fatal_throw_uncaught_previous_mixed: uncaught user exception that wraps an engine DivisionByZeroError as its previous
class WrapError extends Exception {}

function risky($d)
{
    return 10 % $d;
}

function wrap()
{
    try {
        risky(0);
    } catch (DivisionByZeroError $e) {
        throw new WrapError("wrapped engine failure", 5, $e);
    }
}

echo "start\n";
wrap();
echo "never\n";
