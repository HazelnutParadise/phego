<?php
// REQ-018/020; T-035/036/039/040. Resolve against the selected proxy backing.
$tests ??= [];
if (!function_exists('w13R10Attempt')) {
    function w13R10Attempt(callable $f): mixed {
        try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
    }
}
trait W13R10ProxyOperations {
    public static function probe($object, string $operation): mixed {
        switch ($operation) {
            case 'read': return $object->x;
            case 'write': $object->x = 3; return null;
            case 'reference': $alias =& $object->x; $alias = 3; return $alias;
            case 'isset': return isset($object->x);
            case 'unset': unset($object->x); return null;
        }
    }
}
class W13R10ProxyParent {
    use W13R10ProxyOperations;
    protected int $x = 1;
    public int $spare = 2;
    public function peek(): mixed { return $this->x; }
}
class W13R10ProxyChild extends W13R10ProxyParent {
    use W13R10ProxyOperations;
    public int $x = 1;
}
class W13R10ProxyMagicParent {
    use W13R10ProxyOperations;
    protected int $x = 1;
    public int $spare = 2;
    public function peek(): mixed { return $this->x; }
    public function __get($name): mixed { $GLOBALS['w13_r10_proxy_events'][] = 'parent:get:'.$name; return 9; }
    public function __set($name, $value): void { $GLOBALS['w13_r10_proxy_events'][] = 'parent:set:'.$name; }
    public function __isset($name): bool { $GLOBALS['w13_r10_proxy_events'][] = 'parent:isset:'.$name; return true; }
    public function __unset($name): void { $GLOBALS['w13_r10_proxy_events'][] = 'parent:unset:'.$name; }
}
class W13R10ProxyMagicChild extends W13R10ProxyMagicParent {
    use W13R10ProxyOperations;
    public int $x = 1;
    public function __get($name): mixed { $GLOBALS['w13_r10_proxy_events'][] = 'wrapper:get:'.$name; return 9; }
    public function __set($name, $value): void { $GLOBALS['w13_r10_proxy_events'][] = 'wrapper:set:'.$name; }
    public function __isset($name): bool { $GLOBALS['w13_r10_proxy_events'][] = 'wrapper:isset:'.$name; return true; }
    public function __unset($name): void { $GLOBALS['w13_r10_proxy_events'][] = 'wrapper:unset:'.$name; }
}
function w13R10GlobalProxyOperation($object, string $operation): mixed {
    switch ($operation) {
        case 'read': return $object->x;
        case 'write': $object->x = 3; return null;
        case 'reference': $alias =& $object->x; $alias = 3; return $alias;
        case 'isset': return isset($object->x);
        case 'unset': unset($object->x); return null;
    }
}
function w13R10ProxyProbe(bool $magic, bool $warm, string $scope, string $operation): array {
    $GLOBALS['w13_r10_proxy_events'] = [];
    $parent = $magic ? W13R10ProxyMagicParent::class : W13R10ProxyParent::class;
    $child = $magic ? W13R10ProxyMagicChild::class : W13R10ProxyChild::class;
    $backing = new $parent;
    $reflection = new ReflectionClass($child);
    $proxy = $reflection->newLazyProxy(function() use($backing) {
        $GLOBALS['w13_r10_proxy_events'][] = 'factory'; return $backing;
    });
    if ($warm) $GLOBALS['w13_r10_proxy_events'][] = ['warm', $proxy->spare];
    $result = w13R10Attempt(fn() => match ($scope) {
        'parent' => $parent::probe($proxy, $operation),
        'child' => $child::probe($proxy, $operation),
        default => w13R10GlobalProxyOperation($proxy, $operation),
    });
    $peek = w13R10Attempt(fn() => $backing->peek());
    $out = [$result, $peek, $GLOBALS['w13_r10_proxy_events'], $reflection->isUninitializedLazyObject($proxy)];
    unset($proxy, $backing);
    return $out;
}
$w13R10ProxyNames = [];
foreach ([false, true] as $magic) foreach ([false, true] as $warm)
foreach (['global', 'parent', 'child'] as $scope) foreach (['read', 'write', 'reference', 'isset', 'unset'] as $operation) {
    $name = 'r10-proxy-'.($magic ? 'magic' : 'plain').'-'.($warm ? 'warm' : 'cold').'-'.$scope.'-'.$operation;
    $w13R10ProxyNames[] = $name;
    $tests[$name] = fn() => w13R10ProxyProbe($magic, $warm, $scope, $operation);
}
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R10ProxyNames as $name) {
        $diagnostics = [];
        set_error_handler(function($level, $message) use(&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = w13R10Attempt($tests[$name]); restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
