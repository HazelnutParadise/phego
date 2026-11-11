<?php
// REQ-018/020; T-035/036. A ghost default for y must preserve materialized slots.
if (!function_exists('w13R7Attempt')) {
    function w13R7Attempt(callable $f): mixed {
        try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
    }
}
class W13R7DefaultsInt { public int $x = 1; public int $y = 2; public int $z = 3; }
class W13R7DefaultsMixed { public mixed $x = 1; public int $y = 2; public int $z = 3; }
class W13R7DefaultsUntyped { public $x = 1; public int $y = 2; public int $z = 3; }
class W13R7DefaultsNull { public mixed $x = 1; public int $y = 2; public int $z = 3; }
class W13R7DefaultsArray { public array $x = [1]; public int $y = 2; public int $z = 3; }
class W13R7DefaultsObject { public mixed $x = null; public int $y = 2; public int $z = 3; }
class W13R7Payload { public function __destruct() { $GLOBALS['w13_r7_events'][] = 'dtor'; } }

function w13R7Probe(string $name): array {
    $GLOBALS['w13_r7_events'] = [];
    $calls = 0;
    $class = match ($name) {
        'r7-default-int', 'r7-default-unset-typed' => W13R7DefaultsInt::class,
        'r7-default-null' => W13R7DefaultsNull::class,
        'r7-default-array' => W13R7DefaultsArray::class,
        'r7-default-object' => W13R7DefaultsObject::class,
        'r7-default-unset-untyped' => W13R7DefaultsUntyped::class,
        default => W13R7DefaultsMixed::class,
    };
    $r = new ReflectionClass($class);
    $x = $r->getProperty('x'); $z = $r->getProperty('z');
    $object = $name === 'r7-default-object' ? new W13R7Payload : null;
    $init = function ($o) use (&$calls, $name, $r) {
        ++$calls; $GLOBALS['w13_r7_events'][] = ['init', $calls];
        if ($name === 'r7-default-retry-raw' && $calls === 1) {
            $r->getProperty('z')->setRawValueWithoutLazyInitialization($o, 99);
            $GLOBALS['w13_r7_events'][] = ['raw', $r->getProperty('z')->isLazy($o)];
        }
        if (($name === 'r7-default-retry' || $name === 'r7-default-retry-raw') && $calls === 1) { $GLOBALS['w13_r7_events'][] = 'throw'; throw new Exception('retry'); }
        return null;
    };
    $o = $r->newLazyGhost($init);
    $x->setRawValueWithoutLazyInitialization($o, match ($name) {
        'r7-default-null' => null,
        'r7-default-array' => [42],
        'r7-default-object' => $object,
        default => 42,
    });
    if ($name === 'r7-default-unset-typed' || $name === 'r7-default-unset-untyped') unset($o->x);
    if ($name === 'r7-default-retry') { $z->setRawValueWithoutLazyInitialization($o, 3); unset($o->z); }
    $before = $r->isUninitializedLazyObject($o);
    $out = [$before];
    if ($name === 'r7-default-retry' || $name === 'r7-default-retry-raw') {
        $out[] = w13R7Attempt(fn() => $o->y); $out[] = $r->isUninitializedLazyObject($o);
        if ($name === 'r7-default-retry-raw') $out[] = $z->isLazy($o);
        $out[] = w13R7Attempt(fn() => $o->y); $out[] = $r->isUninitializedLazyObject($o);
        $out[] = w13R7Attempt(fn() => $o->x); $out[] = w13R7Attempt(fn() => $o->z);
    } elseif ($name === 'r7-default-reference') {
        $alias =& $o->x; $out[] = w13R7Attempt(fn() => $o->y); $out[] = $r->isUninitializedLazyObject($o);
        $alias = 43; $out[] = $alias; $out[] = $o->x; unset($alias);
    } elseif ($name === 'r7-default-object') {
        $out[] = w13R7Attempt(fn() => $o->y); $out[] = $r->isUninitializedLazyObject($o);
        $out[] = $o->x === $object; $out[] = $o->z;
    } else {
        $out[] = w13R7Attempt(fn() => $o->y); $out[] = $r->isUninitializedLazyObject($o);
        $out[] = w13R7Attempt(fn() => $o->x); $out[] = w13R7Attempt(fn() => $o->z);
    }
    if ($name === 'r7-default-object') {
        $out[] = $GLOBALS['w13_r7_events'];
        unset($object); $out[] = $GLOBALS['w13_r7_events'];
    }
    unset($o);
    if ($name === 'r7-default-object') $out[] = $GLOBALS['w13_r7_events'];
    return [true, $out, $GLOBALS['w13_r7_events']];
}

$w13R7Names = ['r7-default-int','r7-default-null','r7-default-unset-typed','r7-default-unset-untyped','r7-default-array','r7-default-reference','r7-default-object','r7-default-retry','r7-default-retry-raw'];
foreach ($w13R7Names as $name) $tests[$name] = fn() => w13R7Probe($name);
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R7Names as $name) {
        $diagnostics = [];
        set_error_handler(function ($level, $message) use (&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = w13R7Attempt(fn() => w13R7Probe($name)); restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
