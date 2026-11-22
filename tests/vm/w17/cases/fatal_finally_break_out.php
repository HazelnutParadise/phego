<?php
// fatal_finally_break_out: break inside finally that would leave the finally block is a compile error
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "jump out of a finally block is disallowed" on the break line.
// The loop sits outside the try, so the break would leave the finally block early.
// A break that stays inside the finally (inside its own nested loop) is allowed.
// A break or continue in the try block itself is allowed: it runs finally first.
for ($i = 0; $i < 3; $i++) {
    try {
        echo "body\n";
    } finally {
        break;
    }
}
echo "not reached\n";
