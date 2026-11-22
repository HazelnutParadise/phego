<?php
// nope(...) names a function that does not exist. The error comes when the
// closure is created, not when it would be called, so the echo before it runs
// and the echo after it does not.

function exists_one($x)
{
    return $x + 100;
}

$ok = exists_one(...);
echo "existing function: ", $ok(1), "\n";

echo "before the bad first-class callable\n";

$bad = nope(...);

echo "after creation, not reached\n";

$never_called = function () {
    return "never";
};
echo $never_called(), "\n";
