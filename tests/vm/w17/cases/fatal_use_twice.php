<?php
// Compile-time rule: the same variable cannot appear twice in one use list.
// The duplicate sits on its own line, so the error reports that line.
// The whole file fails to compile, so none of the code below runs.

$a = 1;
$b = 2;

$ok = function () use ($a, $b) {
    return $a + $b;
};
echo "single use list: ", $ok(), "\n";

function make_pair()
{
    $p = 4;
    $q = 5;
    return function () use ($p, $q) {
        return $p * $q;
    };
}

$pair = make_pair();
echo "pair: ", $pair(), "\n";

// Invalid: $a is listed twice, on two different lines.
$bad = function () use (
    $a,
    $a
) {
    return $a;
};

echo "not reached\n";
