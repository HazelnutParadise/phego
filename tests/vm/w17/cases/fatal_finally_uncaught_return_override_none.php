<?php
// fatal_finally_uncaught_return_override_none: finally echoes but does not return, so the pending exception goes uncaught
function returns_value()
{
    try {
        return "from try";
    } finally {
        echo "finally with no return\n";
    }
}

$v = returns_value();
echo "returned: ", $v, "\n";
echo "before\n";
try {
    throw new Exception("pending");
} finally {
    echo "finally output\n";
}
echo "not reached\n";
