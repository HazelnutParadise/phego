<?php
// REQ-018/020; T-035/036/039/040. Lazy initialization must not return a released receiver.
$tests ??= [];
if (!function_exists('w13R10Attempt')) {
    function w13R10Attempt(callable $f): mixed {
        try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
    }
}
class W13R10LazyLife {
    public int $x = 42;
    public function __destruct() {
        $GLOBALS['w13_r10_life_events'][] = 'dtor';
        if ($GLOBALS['w13_r10_dtor_throw']) throw new Exception('dtor');
    }
}
function w13R10LazyLifeProbe(string $mode, string $operation, bool $keep, string $failure): array {
    $GLOBALS['w13_r10_life_events'] = [];
    $GLOBALS['w13_r10_dtor_throw'] = $failure === 'dtor';
    $reflection = new ReflectionClass(W13R10LazyLife::class);
    $object = null;
    $lazy = $reflection->newLazyGhost(function($receiver) use($mode, $failure, &$object) {
        $GLOBALS['w13_r10_life_events'][] = 'init';
        if ($mode === 'direct') $object = null;
        else unset($GLOBALS['w13_r10_life']);
        if ($failure === 'init') throw new Exception('init');
    });
    if ($mode === 'direct') $object = $lazy;
    else $GLOBALS['w13_r10_life'] = $lazy;
    unset($lazy);
    $holder = $keep ? ($mode === 'direct' ? $object : $GLOBALS['w13_r10_life']) : null;
    $result = w13R10Attempt(function() use($mode, $operation, &$object) {
        if ($mode === 'direct') {
            switch ($operation) {
                case 'read': return $object->x;
                case 'write': $object->x = 7; return null;
                case 'isset': return isset($object->x);
                case 'unset': unset($object->x); return null;
                case 'reference': $alias =& $object->x; return $alias;
            }
        }
        switch ($operation) {
            case 'read': return $GLOBALS['w13_r10_life']->x;
            case 'write': $GLOBALS['w13_r10_life']->x = 7; return null;
            case 'isset': return isset($GLOBALS['w13_r10_life']->x);
            case 'unset': unset($GLOBALS['w13_r10_life']->x); return null;
            case 'reference': $alias =& $GLOBALS['w13_r10_life']->x; return $alias;
        }
    });
    $out = [$result, $GLOBALS['w13_r10_life_events'], $mode === 'direct' ? isset($object) : isset($GLOBALS['w13_r10_life'])];
    // Retained-control cleanup can throw; keep its diagnostics inside the case.
    $cleanup = w13R10Attempt(function() use(&$holder) { $holder = null; });
    $out[] = $cleanup; $out[] = $GLOBALS['w13_r10_life_events'];
    $GLOBALS['w13_r10_dtor_throw'] = false;
    unset($GLOBALS['w13_r10_life']);
    return $out;
}
$w13R10LifeNames = [];
foreach (['direct', 'globals'] as $mode) foreach (['read', 'write', 'isset', 'unset', 'reference'] as $operation) foreach ([false, true] as $keep) foreach (['none', 'init', 'dtor'] as $failure) {
    $name = 'r10-life-'.$mode.'-'.$operation.'-'.($keep ? 'kept' : 'released').'-'.$failure;
    $w13R10LifeNames[] = $name; $tests[$name] = fn() => w13R10LazyLifeProbe($mode, $operation, $keep, $failure);
}
function w13R10LazyProxyLifeProbe(string $operation, bool $external, string $failure): array {
    $GLOBALS['w13_r10_life_events'] = [];
    $GLOBALS['w13_r10_dtor_throw'] = $failure === 'dtor';
    $reflection = new ReflectionClass(W13R10LazyLife::class);
    $proxy = null; $backing = $external ? new W13R10LazyLife : null;
    $proxy = $reflection->newLazyProxy(function() use(&$proxy, $backing, $failure) {
        $GLOBALS['w13_r10_life_events'][] = 'init'; $proxy = null;
        if ($failure === 'init') throw new Exception('init');
        return $backing ?? new W13R10LazyLife;
    });
    $result = w13R10Attempt(function() use(&$proxy, $operation) {
        switch ($operation) {
            case 'read': return $proxy->x;
            case 'write': $proxy->x = 7; return null;
            case 'isset': return isset($proxy->x);
            case 'unset': unset($proxy->x); return null;
            case 'reference': $alias =& $proxy->x; return $alias;
        }
    });
    $peek = $external ? w13R10Attempt(fn() => $backing->x) : null;
    $out = [$result, $GLOBALS['w13_r10_life_events'], isset($proxy), $peek];
    $out[] = w13R10Attempt(function() use(&$backing) { $backing = null; });
    $out[] = $GLOBALS['w13_r10_life_events'];
    $GLOBALS['w13_r10_dtor_throw'] = false;
    return $out;
}
foreach (['read', 'write', 'isset', 'unset', 'reference'] as $operation) foreach ([false, true] as $external) foreach (['none', 'init', 'dtor'] as $failure) {
    $name = 'r10-life-proxy-'.$operation.'-'.($external ? 'external' : 'ephemeral').'-'.$failure;
    $w13R10LifeNames[] = $name; $tests[$name] = fn() => w13R10LazyProxyLifeProbe($operation, $external, $failure);
}
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R10LifeNames as $name) {
        $diagnostics = [];
        set_error_handler(function($level, $message) use(&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = w13R10Attempt($tests[$name]); restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
