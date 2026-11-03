<?php
// REQ-018; T-035/036. Caller strictness reaches reference-aware property stores.
$tests ??= [];
require_once __DIR__.'/review_r10_strict.php';
if (!function_exists('w13R10Attempt')) {
    function w13R10Attempt(callable $f): mixed {
        try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
    }
}
#[AllowDynamicProperties]
class W13R10Alias {
    public $a = 1;
    public int $b;
    public int $spare = 42;
    public static $s = 1;
    public static int $t;
}
function w13R10WeakInstanceStore($object, string $name, mixed $value): void { $object->$name = $value; }
function w13R10WeakStaticStore(string $class, string $name, mixed $value): void { $class::$$name = $value; }
function w13R10WeakRawStore($object, string $name, mixed $value): void { (new ReflectionProperty($object, $name))->setRawValueWithoutLazyInitialization($object, $value); }
function w13R10AliasProbe(string $storage, bool $strict, string $input): array {
    $calls = 0;
    $reflection = new ReflectionClass(W13R10Alias::class);
    $object = $storage === 'raw-lazy'
        ? $reflection->newLazyGhost(function() use(&$calls) { ++$calls; }) : new W13R10Alias;
    $name = $storage === 'dynamic' ? 'd' : 'a';
    if ($storage === 'static') {
        W13R10Alias::$s = 1; W13R10Alias::$t =& W13R10Alias::$s;
        $result = w13R10Attempt(fn() => $strict ? w13R10StrictStaticStore(W13R10Alias::class, 's', $input) : w13R10WeakStaticStore(W13R10Alias::class, 's', $input));
        $out = [$result, W13R10Alias::$s, W13R10Alias::$t];
        // Reset the shared cells before the next case.
        unset($object); W13R10Alias::$s = 1;
        return $out;
    }
    if ($storage === 'raw-lazy') {
        $reflection->getProperty('a')->setRawValueWithoutLazyInitialization($object, 1);
        $reflection->getProperty('b')->setRawValueWithoutLazyInitialization($object, 1);
    } else $object->$name = 1;
    $object->b =& $object->$name;
    $result = w13R10Attempt(fn() => str_starts_with($storage, 'raw')
        ? ($strict ? w13R10StrictRawStore($object, $name, $input) : w13R10WeakRawStore($object, $name, $input))
        : ($strict ? w13R10StrictInstanceStore($object, $name, $input) : w13R10WeakInstanceStore($object, $name, $input)));
    if (str_starts_with($storage, 'raw')) {
        $out = [$result, $object->$name, $object->b, $calls, $reflection->isUninitializedLazyObject($object)];
        unset($object); return $out;
    }
    $out = [$result, $object->$name, $object->b]; unset($object); return $out;
}
$w13R10AliasNames = [];
foreach (['declared', 'dynamic', 'static', 'raw', 'raw-lazy'] as $storage) foreach ([false, true] as $strict) foreach (['2', 'bad'] as $input) {
    $name = 'r10-alias-'.$storage.'-'.($strict ? 'strict' : 'weak').'-'.($input === '2' ? 'numeric' : 'bad');
    $w13R10AliasNames[] = $name; $tests[$name] = fn() => w13R10AliasProbe($storage, $strict, $input);
}
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R10AliasNames as $name) {
        $diagnostics = [];
        set_error_handler(function($level, $message) use(&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = w13R10Attempt($tests[$name]); restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
