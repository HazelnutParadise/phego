<?php
// Emits PHP's own results for conversions, arithmetic, and comparison over a
// fixed operand set. Values are encoded exactly: floats by their IEEE-754
// bits, strings by hex. Run with `php -n` in a pinned reference image.
require __DIR__ . '/enc.php';
$operands = [
    null, true, false, 0, 1, -1, 2, 7, -7, 63, 64, PHP_INT_MAX, PHP_INT_MIN,
    0.0, -0.0, 0.1, 0.5, 1.5, -1.5, 3.0, 1e20, 1e100, INF, -INF, NAN,
    "", "0", "1", "-1", "00", "1.5", " 1", "1 ", "1abc", "abc", "0x1A", "1e3",
    ".5", "9223372036854775808", "  ", "1_000", "INF", "a", "b",
    [], [1], [1, 2], ["a" => 1],
];
$binary = [
    '+' => fn($a, $b) => $a + $b, '-' => fn($a, $b) => $a - $b,
    '*' => fn($a, $b) => $a * $b, '/' => fn($a, $b) => $a / $b,
    '%' => fn($a, $b) => $a % $b, '**' => fn($a, $b) => $a ** $b,
    '.' => fn($a, $b) => $a . $b,
    '==' => fn($a, $b) => $a == $b, '!=' => fn($a, $b) => $a != $b,
    '===' => fn($a, $b) => $a === $b, '<' => fn($a, $b) => $a < $b,
    '<=' => fn($a, $b) => $a <= $b, '>' => fn($a, $b) => $a > $b,
    '>=' => fn($a, $b) => $a >= $b, '<=>' => fn($a, $b) => $a <=> $b,
    '&' => fn($a, $b) => $a & $b, '|' => fn($a, $b) => $a | $b,
    '^' => fn($a, $b) => $a ^ $b, '<<' => fn($a, $b) => $a << $b,
    '>>' => fn($a, $b) => $a >> $b,
];
$unary = [
    '(int)' => fn($a) => (int)$a, '(float)' => fn($a) => (float)$a,
    '(string)' => fn($a) => (string)$a, '(bool)' => fn($a) => (bool)$a,
    'neg' => fn($a) => -$a, 'pos' => fn($a) => +$a, '~' => fn($a) => ~$a,
    '!' => fn($a) => !$a,
];
$diags = [];
set_error_handler(function ($no, $msg) use (&$diags) { $diags[] = [$no, $msg]; return true; });
function run(callable $f, array $args, array &$diags) {
    $diags = [];
    try {
        $r = ['r' => enc($f(...$args))];
    } catch (Throwable $e) {
        $r = ['e' => [get_class($e), $e->getMessage()]];
    }
    if ($diags) { $r['d'] = $diags; }
    return $r;
}
$out = ['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'operands' => array_map('enc', $operands), 'unary' => [], 'binary' => []];
foreach ($unary as $name => $f) {
    foreach ($operands as $a) { $out['unary'][$name][] = run($f, [$a], $diags); }
}
foreach ($binary as $name => $f) {
    foreach ($operands as $a) {
        foreach ($operands as $b) { $out['binary'][$name][] = run($f, [$a, $b], $diags); }
    }
}
echo json_encode($out, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
