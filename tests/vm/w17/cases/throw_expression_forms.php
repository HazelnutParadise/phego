<?php
// throw_expression_forms: throw used as an expression in ??, ?:, ||, &&, arrow functions, match arms and ternaries
$nothing = null;
try {
    $v1 = $nothing ?? throw new Exception("coalesce");
    echo "not reached 1\n";
} catch (Exception $e) {
    echo "coalesce: ", $e->getMessage(), "\n";
}

$five = 5;
$v2 = $five ?? throw new Exception("not thrown");
echo "coalesce_value_kept: ", $v2, "\n";

$zero = 0;
try {
    $v3 = $zero ?: throw new Exception("elvis");
    echo "not reached 3\n";
} catch (Exception $e) {
    echo "elvis: ", $e->getMessage(), "\n";
}

$ok = true;
$bad = false;
$ok || throw new Exception("or_not_thrown");
echo "or_ok_short_circuit: 1\n";
try {
    $bad || throw new Exception("or_bad");
    echo "not reached 4\n";
} catch (Exception $e) {
    echo "or_bad: ", $e->getMessage(), "\n";
}

try {
    $ok && throw new Exception("and_ok");
    echo "not reached 5\n";
} catch (Exception $e) {
    echo "and_ok: ", $e->getMessage(), "\n";
}
$bad && throw new Exception("and_not_thrown");
echo "and_bad_short_circuit: 1\n";

$fn = fn($x) => $x ?? throw new Exception("arrow_" . $x);
echo "arrow_value: ", $fn(7), "\n";
try {
    $fn(null);
} catch (Exception $e) {
    echo "arrow_thrown: ", $e->getMessage(), "\n";
}

$k = 1;
echo "match_one: ", match ($k) { 1 => "one", default => throw new Exception("match default") }, "\n";
$k = 2;
try {
    $m = match ($k) { 1 => "one", default => throw new Exception("match default") };
} catch (Exception $e) {
    echo "match_default: ", $e->getMessage(), "\n";
}

$t1 = $ok ? "yes" : throw new Exception("ternary true");
echo "ternary_true: ", $t1, "\n";
try {
    $t2 = $bad ? "yes" : throw new Exception("ternary false");
} catch (Exception $e) {
    echo "ternary_false: ", $e->getMessage(), "\n";
}
echo "end\n";
