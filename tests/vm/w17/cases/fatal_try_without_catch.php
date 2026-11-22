<?php
// fatal_try_without_catch: a try block with neither catch nor finally is a compile error
// Nothing is printed, even the first echo, because the parser rejects the file.
// The parser rejects a try with no catch and no finally, so this is not a runtime condition.
// The statement after the try block is never executed, because the file never compiles.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "Cannot use try without catch or finally" on the line of the try statement.
// A valid form needs at least one clause, for example try { ... } catch (Exception $e) { ... }
// or try { ... } finally { ... }. Both are accepted by the parser.
// The statements after the broken try are kept here only to show the file would otherwise run.
echo "before\n";
try {
    echo "inside\n";
}
echo "after\n";
