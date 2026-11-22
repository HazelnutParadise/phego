<?php
// Compile-time rule: an auto-global such as $_GET cannot be a lexical variable.
// The whole file fails to compile, so none of the code below runs.

function scale($n)
{
    return function ($m) use ($n) {
        return $n * $m;
    };
}

$double = scale(2);
echo "scaled: ", $double(21), "\n";

$triple = scale(3);
echo "scaled again: ", $triple(5), "\n";

$names = ["first", "second"];
$pick = function ($i) use ($names) {
    return $names[$i];
};
echo "picked: ", $pick(1), "\n";

// Invalid: $_GET is an auto-global, not a local variable.
$bad = function () use ($_GET) {
    return 0;
};

echo "not reached\n";
