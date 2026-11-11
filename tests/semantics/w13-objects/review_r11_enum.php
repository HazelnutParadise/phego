<?php
// REQ-018; T-035/036. Enum scalar comparisons are unordered except bool/null.
$tests ??= [];
if (!function_exists('w13R11Attempt')) {
    function w13R11Attempt(callable $f): mixed { try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; } }
}
enum W13R11Unit { case A; case B; }
enum W13R11Int: int { case A = 1; case B = 2; }
enum W13R11String: string { case A = 'a'; case B = 'b'; }
class W13R11Ordinary {}
$w13R11EnumNames = [];
foreach (['unit', 'int', 'string'] as $kind) foreach (['zero', 'one', 'two', 'minus-one', 'float-one', 'float-two', 'nan', 'inf', 'negative-inf', 'empty-string', 'numeric-string', 'text', 'empty-array', 'array', 'false', 'true', 'null', 'object', 'resource', 'same', 'other'] as $rhs) {
    $name = 'r11-enum-'.$kind.'-'.$rhs; $w13R11EnumNames[] = $name;
    $tests[$name] = function() use($kind, $rhs) {
        $class = match($kind) { 'unit' => W13R11Unit::class, 'int' => W13R11Int::class, 'string' => W13R11String::class };
        $left = $class::A;
        $right = match($rhs) {
            'zero' => 0, 'one' => 1, 'two' => 2, 'minus-one' => -1,
            'float-one' => 1.0, 'float-two' => 2.0, 'nan' => NAN, 'inf' => INF, 'negative-inf' => -INF,
            'empty-string' => '', 'numeric-string' => '1', 'text' => 'a',
            'empty-array' => [], 'array' => [1], 'false' => false, 'true' => true, 'null' => null,
            'object' => new W13R11Ordinary, 'resource' => fopen('php://memory', 'r+'), 'same' => $class::A, 'other' => $class::B,
        };
        try { return [$left == $right, $left <=> $right, $right <=> $left]; }
        finally { if (is_resource($right)) fclose($right); }
    };
}
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R11EnumNames as $name) {
        $diagnostics = [];
        set_error_handler(function($level, $message) use(&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = w13R11Attempt($tests[$name]); restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
