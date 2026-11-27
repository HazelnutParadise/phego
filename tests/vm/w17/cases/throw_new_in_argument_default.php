<?php
// throw_new_in_argument_default: a function with a default argument throws a message built from that argument, called with default and named arguments
function f($a = 1) {
    throw new LogicException("x" . $a);
}

function g($label = "none", $count = 3) {
    throw new DomainException("g " . $label . " " . $count, $count);
}

try {
    f();
    echo "not reached 1\n";
} catch (LogicException $e) {
    echo "default_message: ", $e->getMessage(), "\n";
    echo "default_class: ", $e::class, "\n";
}

try {
    f(a: 7);
    echo "not reached 2\n";
} catch (LogicException $e) {
    echo "named_message: ", $e->getMessage(), "\n";
}

try {
    f(5);
} catch (LogicException $e) {
    echo "positional_message: ", $e->getMessage(), "\n";
}

try {
    g();
} catch (DomainException $e) {
    echo "g_default: ", $e->getMessage(), "\n";
    echo "g_code: ", $e->getCode(), "\n";
}

try {
    g(count: 9);
} catch (DomainException $e) {
    echo "g_named_only_count: ", $e->getMessage(), "\n";
    echo "g_named_code: ", $e->getCode(), "\n";
}

try {
    g("left", 4);
} catch (DomainException $e) {
    echo "g_positional: ", $e->getMessage(), "\n";
}
echo "end\n";
