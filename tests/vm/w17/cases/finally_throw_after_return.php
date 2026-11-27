<?php
// finally_throw_after_return: try returns a value, finally throws; the caller sees the exception, not the value
class Late extends Exception {}

function tryReturns()
{
    try {
        echo "tryReturns try\n";
        return "value from try";
    } finally {
        echo "tryReturns finally throws\n";
        throw new Late("late exception");
    }
}

try {
    $v = tryReturns();
    echo "value: ", $v, "\n";
} catch (Late $e) {
    echo "caller sees: ", $e->getMessage(), "\n";
    echo "value variable unset: ", isset($v) ? "no" : "yes", "\n";
}
echo "end\n";
