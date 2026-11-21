<?php
// Uncaught error three closures deep. The stack trace must show how PHP names
// each closure frame: {closure:FILE:LINE} for closures and the same for arrow functions.

function make_inner()
{
    return function ($v) {
        $arrow = fn($w) => missing_helper($v + $w);
        return $arrow(10);
    };
}

$middle = make_inner();

$outer = function ($start) use ($middle) {
    return $middle($start);
};

echo "tracing starts\n";
echo "outer returns: ", $outer(1), "\n";

echo "not reached\n";
