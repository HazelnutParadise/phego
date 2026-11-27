<?php
// throw_non_throwable: throwing values that are not Throwable objects produces an Error
class Plain {}

try {
    throw 5;
} catch (Error $e) {
    echo "int: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw "text";
} catch (Error $e) {
    echo "string: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw null;
} catch (Error $e) {
    echo "null: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw [1, 2];
} catch (Error $e) {
    echo "array: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw new stdClass();
} catch (Error $e) {
    echo "stdclass: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw new Plain();
} catch (Error $e) {
    echo "user_plain: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw false;
} catch (Error $e) {
    echo "bool: ", $e::class, " ", $e->getMessage(), "\n";
}

try {
    throw 1.5;
} catch (Error $e) {
    echo "float: ", $e::class, " ", $e->getMessage(), "\n";
    echo "float_code: ", $e->getCode(), "\n";
}

class Plain
{
}

try {
    throw new Plain();
} catch (Error $e) {
    echo "plain_object: ", $e::class, " ", $e->getMessage(), "\n";
}

$caught = 0;
try {
    throw 9;
} catch (Throwable $t) {
    $caught = $caught + 1;
    echo "throwable_caught_engine_error: ", $t::class, "\n";
}
echo "caught_count_is_one: ", ($caught === 1 ? 1 : 0), "\n";
echo "end\n";
