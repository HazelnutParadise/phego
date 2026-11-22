<?php
// fatal_throw_uncaught_catch_mismatch: an exception passes several non-matching catch clauses, including unknown classes, and is then uncaught
echo "start\n";
$kept = new RuntimeException("kept");
echo "kept class: ", $kept::class, "\n";
try {
    throw new RuntimeException("mismatch");
} catch (LogicException $e) {
    echo "wrong logic\n";
} catch (NoSuchClass $e) {
    echo "wrong unknown\n";
} catch (Error $e) {
    echo "wrong error\n";
}
echo "never\n";
