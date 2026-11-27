<?php
// exception_ctor_type_errors: TypeError raised by bad constructor arguments of built-in exception classes
try {
    $x1 = new Exception([]);
    echo "no error 1\n";
} catch (TypeError $t) {
    echo "t1_class: ", $t::class, "\n";
    echo "t1_message: ", $t->getMessage(), "\n";
}

try {
    $x2 = new Exception("a", "b");
    echo "no error 2\n";
} catch (TypeError $t) {
    echo "t2_class: ", $t::class, "\n";
    echo "t2_message: ", $t->getMessage(), "\n";
}

try {
    $x3 = new Exception("a", 1, 5);
    echo "no error 3\n";
} catch (TypeError $t) {
    echo "t3_class: ", $t::class, "\n";
    echo "t3_message: ", $t->getMessage(), "\n";
}

try {
    $x4 = new Error([]);
    echo "no error 4\n";
} catch (TypeError $t) {
    echo "t4_class: ", $t::class, "\n";
    echo "t4_message: ", $t->getMessage(), "\n";
}

try {
    $x5 = new Error("a", "xyz");
    echo "no error 5\n";
} catch (TypeError $t) {
    echo "t5_class: ", $t::class, "\n";
    echo "t5_message: ", $t->getMessage(), "\n";
}

try {
    $x6 = new ErrorException("a", 0, "severe");
    echo "no error 6\n";
} catch (TypeError $t) {
    echo "t6_class: ", $t::class, "\n";
    echo "t6_message: ", $t->getMessage(), "\n";
}

try {
    $x7 = new Exception("a", 1, new stdClass());
    echo "no error 7\n";
} catch (TypeError $t) {
    echo "t7_class: ", $t::class, "\n";
    echo "t7_message: ", $t->getMessage(), "\n";
}

$ok = new Exception("ok", "12");
echo "numeric_string_code_accepted: ", $ok->getCode(), "\n";
echo "numeric_string_code_is_int: ", ($ok->getCode() === 12 ? 1 : 0), "\n";
echo "end\n";
