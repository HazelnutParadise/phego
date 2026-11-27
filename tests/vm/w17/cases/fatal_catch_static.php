<?php
// fatal_catch_static: static is not a valid class name in a catch clause (compile error)
// Nothing is printed, because the file fails to compile.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "Bad class name in the catch statement" on the catch line.
// static is a keyword that names the late-bound class, not a class that can be caught.
// A valid catch names a class such as Exception, \Exception, or a namespaced class.
// The variable $e is fine; only the class position is invalid here.
echo "before\n";
try {
    throw new Exception("x");
} catch (static $e) {
    echo "caught\n";
}
echo "not reached\n";
