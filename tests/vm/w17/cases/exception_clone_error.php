<?php
// exception_clone_error: cloning an exception object raises an Error and leaves the original usable
class Sub extends RuntimeException {}

$e = new Exception("original");
try {
    $copy = clone $e;
    echo "cloned\n";
} catch (Error $err) {
    echo "clone_class: ", $err::class, "\n";
    echo "clone_message: ", $err->getMessage(), "\n";
}
echo "original_still: ", $e->getMessage(), "\n";

$s = new Sub("sub");
try {
    $c2 = clone $s;
    echo "not reached sub\n";
} catch (Error $err2) {
    echo "sub_clone_class: ", $err2::class, "\n";
    echo "sub_clone_message: ", $err2->getMessage(), "\n";
}

$r = new Error("err");
try {
    $c3 = clone $r;
    echo "not reached error\n";
} catch (Error $err3) {
    echo "error_clone_message: ", $err3->getMessage(), "\n";
}

$t = new TypeError("te");
try {
    $c4 = clone $t;
    echo "not reached type\n";
} catch (Error $err4) {
    echo "type_clone_class: ", $err4::class, "\n";
}

echo "original_still_throwable: ", ($e instanceof Throwable ? 1 : 0), "\n";
echo "sub_message_after: ", $s->getMessage(), "\n";
echo "end\n";
