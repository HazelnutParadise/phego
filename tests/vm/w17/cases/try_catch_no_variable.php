<?php
// try_catch_no_variable: catch without a variable (PHP 8), alone and in a union
class QuietError extends Exception {}

function fail($n)
{
    if ($n === 0) {
        throw new QuietError("q");
    }
    if ($n === 1) {
        throw new TypeError("t");
    }
    if ($n === 2) {
        throw new Exception("e");
    }
    return "none";
}

echo "step 1\n";
try {
    fail(0);
    echo "not reached\n";
} catch (QuietError) {
    echo "caught QuietError without variable\n";
}

echo "step 2\n";
try {
    fail(1);
} catch (Exception) {
    echo "wrong: Exception clause\n";
} catch (TypeError) {
    echo "caught TypeError without variable\n";
}

echo "step 3\n";
try {
    fail(2);
} catch (TypeError | Exception) {
    echo "union without variable caught Exception\n";
}

echo "step 4\n";
$ok = fail(3);
echo "result: ", $ok, "\n";

echo "step 5\n";
try {
    throw new QuietError("second");
} catch (Error | QuietError) {
    echo "caught by union without variable\n";
}

echo "variable e isset: ", isset($e) ? "yes" : "no", "\n";
