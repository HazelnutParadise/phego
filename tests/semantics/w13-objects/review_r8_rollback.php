<?php
// REQ-018; T-035/036. Lazy rollback preserves observable typed-source order.
$tests ??= [];

if (!function_exists('w13R8Attempt')) {
    function w13R8Attempt(callable $f): mixed {
        try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
    }
}

class W13R8BaseTyped {
    private int $hidden = 9;
    public int $a;
}
class W13R8Typed extends W13R8BaseTyped {
    public int $a = 7;
    public static int $s = 7;
    public int $v { get => 99; }
    public $victim = null;
    public int $b = 2;
    public int $trigger = 1;
}
class W13R8BaseUntyped {
    private int $hidden = 9;
    public $a;
}
class W13R8Untyped extends W13R8BaseUntyped {
    public $a = 7;
    public static int $s = 7;
    public int $v { get => 99; }
    public $victim = null;
    public int $b = 2;
    public int $trigger = 1;
}
class W13R8Token {
    public function __destruct() {
        $GLOBALS['w13_r8_events'][] = ['globalAlias', w13R8Attempt(function() { $GLOBALS['w13_r8_globalAlias'] = 'bad'; })];
    }
}

function w13R8Probe(bool $typed): array {
    $GLOBALS['w13_r8_events'] = [];
    $calls = 0;
    $class = $typed ? W13R8Typed::class : W13R8Untyped::class;
    $r = new ReflectionClass($class);
    $a = $r->getProperty('a');
    $b = $r->getProperty('b');
    $object = $r->newLazyGhost(function ($object) use (&$calls) {
        ++$calls;
        $GLOBALS['w13_r8_events'][] = ['init', $calls];
        $object->victim = new W13R8Token();
        throw new Exception('init');
    });
    $a->setRawValueWithoutLazyInitialization($object, 1);
    $b->setRawValueWithoutLazyInitialization($object, 1);
    $alias =& $object->a;
    $object->b =& $alias;
    $GLOBALS['w13_r8_globalAlias'] =& $alias;

    $attemptResults = [];
    for ($i = 0; $i < 2; ++$i) {
        $attemptResults[] = w13R8Attempt(fn() => $object->trigger);
        $attemptResults[] = w13R8Attempt(function() use(&$alias) { $alias = 'bad'; });
    }
    $hidden = (new ReflectionClass($typed ? W13R8BaseTyped::class : W13R8BaseUntyped::class))->getProperty('hidden');
    $values = [$a->getRawValue($object), $b->getRawValue($object), $alias];
    $result = [$attemptResults, $GLOBALS['w13_r8_events'], $values,
        $r->isUninitializedLazyObject($object), $hidden->isLazy($object), $calls];
    unset($GLOBALS['w13_r8_globalAlias'], $alias, $object);
    return $result;
}

$w13R8Names = ['r8-rollback-sources-typed', 'r8-rollback-sources-untyped'];
$tests['r8-rollback-sources-typed'] = fn() => w13R8Probe(true);
$tests['r8-rollback-sources-untyped'] = fn() => w13R8Probe(false);

if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R8Names as $name) {
        $diagnostics = [];
        set_error_handler(function ($level, $message) use (&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = w13R8Attempt($tests[$name]);
        restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
