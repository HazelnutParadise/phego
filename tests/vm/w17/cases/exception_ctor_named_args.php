<?php
// exception_ctor_named_args: named arguments to exception constructors, and the Error raised for an unknown name
$p = new Exception("inner", 1);

$a = new Exception(code: 3, message: "x", previous: $p);
echo "named_message: ", $a->getMessage(), "\n";
echo "named_code: ", $a->getCode(), "\n";
echo "named_previous_msg: ", $a->getPrevious()->getMessage(), "\n";

$b = new Exception(previous: $p);
echo "only_previous_message: [", $b->getMessage(), "]\n";
echo "only_previous_code: ", $b->getCode(), "\n";
echo "only_previous_is_p: ", ($b->getPrevious() === $p ? 1 : 0), "\n";

$c = new Error(code: 9, message: "err named");
echo "error_named_message: ", $c->getMessage(), "\n";
echo "error_named_code: ", $c->getCode(), "\n";

try {
    $d = new Exception(colour: "red");
    echo "not reached\n";
} catch (Error $err) {
    echo "unknown_class: ", $err::class, "\n";
    echo "unknown_message: ", $err->getMessage(), "\n";
}

$e = new Exception("pos", code: 5);
echo "mixed_message: ", $e->getMessage(), "\n";
echo "mixed_code: ", $e->getCode(), "\n";

try {
    $f = new Exception("pos", message: "again");
    echo "not reached 2\n";
} catch (Error $err2) {
    echo "overwrite_class: ", $err2::class, "\n";
    echo "overwrite_message: ", $err2->getMessage(), "\n";
}

$g = new Exception(previous: $p, message: "named both");
echo "reversed_named_message: ", $g->getMessage(), "\n";
echo "reversed_named_prev_is_p: ", ($g->getPrevious() === $p ? 1 : 0), "\n";
