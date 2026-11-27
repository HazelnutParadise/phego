<?php
// throw_statement_basic: a throw inside a function is caught by the caller and skips the rest of the function
function check_value($v) {
    echo "check start: ", $v, "\n";
    if ($v < 0) {
        throw new InvalidArgumentException("negative: " . $v);
        echo "never printed\n";
    }
    echo "check ok: ", $v, "\n";
    return $v * 2;
}

function nested_throw() {
    throw new Exception("from nested");
    return 1;
}

$first = check_value(3);
echo "first_result: ", $first, "\n";

try {
    $second = check_value(-4);
    echo "not reached: ", $second, "\n";
} catch (InvalidArgumentException $e) {
    echo "caught: ", $e->getMessage(), "\n";
    echo "caught_class: ", $e::class, "\n";
}
echo "after try\n";

try {
    nested_throw();
} catch (Exception $e2) {
    echo "nested_caught: ", $e2->getMessage(), "\n";
    echo "nested_line_in_function: ", $e2->getLine(), "\n";
}

try {
    echo "before throw\n";
    throw new RuntimeException("direct");
    echo "not printed\n";
} catch (RuntimeException $e3) {
    echo "direct_caught: ", $e3->getMessage(), "\n";
}

$total = 0;
foreach ([1, 2, -3, 4] as $v) {
    try {
        $total = $total + check_value($v);
    } catch (InvalidArgumentException $e4) {
        echo "loop_skip: ", $e4->getMessage(), "\n";
    }
}
echo "total: ", $total, "\n";
