<?php
// REQ-018/020; T-035/036/039/040. Proxy resolution preserves static-as-instance diagnostics.
class W13R10StaticParent { public int $spare = 2; public static int $x = 1; }
class W13R10StaticChild extends W13R10StaticParent {}
$w13R10ProxyStaticNames = [];
foreach ([false, true] as $warm) foreach (['read', 'write', 'reference', 'isset', 'unset'] as $operation) {
    $name = 'r10-proxy-static-'.($warm ? 'warm' : 'cold').'-'.$operation;
    $w13R10ProxyStaticNames[] = $name;
    $tests[$name] = function() use($warm, $operation) {
        $events = []; $backing = new W13R10StaticParent;
        $reflection = new ReflectionClass(W13R10StaticChild::class);
        $proxy = $reflection->newLazyProxy(function() use($backing, &$events) { $events[] = 'factory'; return $backing; });
        if ($warm) $events[] = ['warm', $proxy->spare];
        $result = w13R10Attempt(function() use($proxy, $operation) {
            switch ($operation) {
                case 'read': return $proxy->x;
                case 'write': $proxy->x = 3; return null;
                case 'reference': $alias =& $proxy->x; $alias = 3; return $alias;
                case 'isset': return isset($proxy->x);
                case 'unset': unset($proxy->x); return null;
            }
        });
        return [$result, $events, $reflection->isUninitializedLazyObject($proxy)];
    };
}


class W13R10StaticMagicParent extends W13R10StaticParent {
    public function __get($name): mixed { $GLOBALS['w13_r10_static_magic_events'][] = 'parent:get:'.$name; return 9; }
    public function __set($name, $value): void { $GLOBALS['w13_r10_static_magic_events'][] = 'parent:set:'.$name; }
    public function __isset($name): bool { $GLOBALS['w13_r10_static_magic_events'][] = 'parent:isset:'.$name; return true; }
    public function __unset($name): void { $GLOBALS['w13_r10_static_magic_events'][] = 'parent:unset:'.$name; }
}
class W13R10StaticMagicChild extends W13R10StaticMagicParent {
    public function __get($name): mixed { $GLOBALS['w13_r10_static_magic_events'][] = 'wrapper:get:'.$name; return 9; }
    public function __set($name, $value): void { $GLOBALS['w13_r10_static_magic_events'][] = 'wrapper:set:'.$name; }
    public function __isset($name): bool { $GLOBALS['w13_r10_static_magic_events'][] = 'wrapper:isset:'.$name; return true; }
    public function __unset($name): void { $GLOBALS['w13_r10_static_magic_events'][] = 'wrapper:unset:'.$name; }
}
foreach ([false, true] as $warm) foreach (['read', 'write', 'reference', 'isset', 'unset'] as $operation) {
    $name = 'r10-proxy-staticmagic-'.($warm ? 'warm' : 'cold').'-'.$operation;
    $tests[$name] = function() use($warm, $operation) {
        $GLOBALS['w13_r10_static_magic_events'] = [];
        $backing = new W13R10StaticMagicParent; $reflection = new ReflectionClass(W13R10StaticMagicChild::class);
        $proxy = $reflection->newLazyProxy(function() use($backing) { $GLOBALS['w13_r10_static_magic_events'][] = 'factory'; return $backing; });
        if ($warm) $GLOBALS['w13_r10_static_magic_events'][] = ['warm', $proxy->spare];
        $result = w13R10Attempt(function() use($proxy, $operation) {
            switch ($operation) {
                case 'read': return $proxy->x;
                case 'write': $proxy->x = 3; return null;
                case 'reference': $alias =& $proxy->x; $alias = 3; return $alias;
                case 'isset': return isset($proxy->x);
                case 'unset': unset($proxy->x); return null;
            }
        });
        return [$result, $GLOBALS['w13_r10_static_magic_events'], $reflection->isUninitializedLazyObject($proxy)];
    };
}
class W13R10StaticGuard extends W13R10StaticParent {
    public function __get($name): mixed { $GLOBALS['w13_r10_guard_events'][] = 'get'; return $this->$name; }
    public function __set($name, $value): void { $GLOBALS['w13_r10_guard_events'][] = 'set'; $this->$name = $value; }
}
foreach ([false, true] as $dynamic) {
    $tests['r10-staticmagic-'.($dynamic ? 'dynamic' : 'guarded').'-read'] = function() use($dynamic) {
        $GLOBALS['w13_r10_guard_events'] = []; $object = new W13R10StaticGuard;
        if ($dynamic) $object->x = 5;
        $result = w13R10Attempt(fn() => $object->x);
        return [$result, $GLOBALS['w13_r10_guard_events']];
    };
}
