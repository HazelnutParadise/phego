<?php
// fatal_finally_continue_out: continue inside finally that would leave the finally block is a compile error
// Nothing is printed, even the body echo, because the file fails to compile.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "jump out of a finally block is disallowed" on the continue line.
// The foreach loop sits outside the try, so the continue would leave the finally block early.
// A continue in the try block itself is allowed: it runs finally first.
foreach ([1, 2] as $v) {
    try {
        echo "body\n";
    } finally {
        continue;
    }
}
echo "not reached\n";
