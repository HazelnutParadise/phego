<?php
// fatal_throw_uncaught_rethrown: an exception is caught, reported and rethrown; the reported line is where it was created
function make()
{
    return new Exception("made here");
}

function pass_it()
{
    try {
        throw make();
    } catch (Exception $e) {
        echo "caught once\n";
        throw $e;
    }
}

echo "start\n";
pass_it();
echo "never\n";
