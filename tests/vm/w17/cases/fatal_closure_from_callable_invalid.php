<?php
// Closure::fromCallable with a name that is not a function. PHP raises a
// type error whose message says why the closure could not be made.

function known($a)
{
    return "known " . $a;
}

$k = Closure::fromCallable('known');
echo "from a known function: ", $k("x"), "\n";

$f = Closure::fromCallable(function () {
    return "already a closure";
});
echo "from a closure: ", $f(), "\n";

$same = Closure::fromCallable($k);
echo "same object: ", $same === $k ? "same" : "different", "\n";

echo "before the invalid name\n";

$bad = Closure::fromCallable('nope');

echo "after the call, not reached\n";
