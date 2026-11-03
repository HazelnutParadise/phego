<?php
// REQ-018/020; T-035/036/039/040. R6 lazy unset priority:
// descriptor-only denials precede lazy initialization, while an
// explicitly-unset property still reaches magic __unset.
if (!function_exists('attempt')) {
    function attempt(callable $f): mixed {
        try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
    }
}
function w13R6Event(string $event): void { $GLOBALS['w13_r6_events'][] = $event; }
function w13R6Ghost(string $class, callable $initializer, callable $body): array {
    $reflection = new ReflectionClass($class);
    $GLOBALS['w13_r6_events'] = [];
    $GLOBALS['w13_r6_calls'] = 0;
    $object = $reflection->newLazyGhost(function ($object) use ($initializer) {
        $GLOBALS['w13_r6_calls']++;
        return $initializer($object);
    });
    $before = $reflection->isUninitializedLazyObject($object);
    $result = $body($object, $reflection);
    return [$before, $result, $GLOBALS['w13_r6_events']];
}
class W13R6Virtual {
    public int $q = 0;
    public int $v { get { w13R6Event('get'); return $this->q + 1; } }
}
class W13R6Backed {
    public int $q = 0;
    public int $b = 1 {
        get { w13R6Event('get'); return $this->b; }
        set { w13R6Event('set'); $this->b = $value; }
    }
}
class W13R6PrivSet { public private(set) int $p; public int $q = 0; }
class W13R6ProtSet { public protected(set) int $p; public int $q = 0; }
class W13R6RoUninit { public readonly int $ro; public int $q = 0; }
class W13R6RoRaw { public readonly int $ro; public int $q = 0; }
class W13R6ReadonlyInit {
    public readonly int $ro;
    public int $q = 0;
    public function initializeReadonly(): void { $this->ro = 123; }
    public function clearReadonly(): void { unset($this->ro); }
}
class W13R6PrivateHook {
    public int $q = 0;
    public private(set) int $p { get { return $this->q; } set { $this->q = $value; } }
}
class W13R6Plain { public int $q; }
class W13R6RawPlain { public int $q; public int $spare; }
class W13R6DynamicMagic {
    public int $q = 0;
    public function __unset($name): void { w13R6Event('magic:' . $name); }
}
class W13R6DynamicPlain { public int $q = 0; }
class W13R6ThrowPlain { public int $q; }
class W13R6MagicAsym {
    public private(set) int $p;
    public int $q = 0;
    public function __construct() { $this->p = 7; unset($this->p); }
    public function __unset($name): void { w13R6Event('magic:' . $name); }
}
class W13R6RecurParent {
    public function __unset($name): void { w13R6Event('magic:' . $name); unset($this->p); }
}
class W13R6RecurChild extends W13R6RecurParent {
    public private(set) int $p;
    public int $q = 0;
    public function clear(): void { unset($this->p); }
}
class W13R6RoClone {
    public readonly int $ro;
    public function __construct(int $value) { $this->ro = $value; }
    public function __clone() { w13R6Event('clone'); unset($this->ro); $this->ro = 42; }
}
class W13R6RoMagic {
    public readonly int $ro;
    public int $q = 0;
    public function __unset($name): void { w13R6Event('magic:' . $name); }
    public function clear(): void { unset($this->ro); }
}
$w13R6Tests = [];
$w13R6Tests['r6-lazy-virtual-unset'] = function () {
    return w13R6Ghost(W13R6Virtual::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $property = $reflection->getProperty('v');
            $virtual = $property->isVirtual();
            $unset = attempt(function () use ($object) { unset($object->v); });
            return [$virtual, $unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-backed-unset'] = function () {
    return w13R6Ghost(W13R6Backed::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $property = $reflection->getProperty('b');
            $virtual = $property->isVirtual();
            $unset = attempt(function () use ($object) { unset($object->b); });
            return [$virtual, $unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-private-set-unset'] = function () {
    return w13R6Ghost(W13R6PrivSet::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->p); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-protected-set-unset'] = function () {
    return w13R6Ghost(W13R6ProtSet::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->p); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-readonly-uninit-unset'] = function () {
    return w13R6Ghost(W13R6RoUninit::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->ro); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-readonly-raw-unset'] = function () {
    return w13R6Ghost(W13R6RoRaw::class,
        function ($object) { w13R6Event('init'); $object->q = 9; },
        function ($object, ReflectionClass $reflection) {
            $property = $reflection->getProperty('ro');
            $property->setRawValueWithoutLazyInitialization($object, 123);
            $raw = $property->getRawValue($object);
            $unset = attempt(function () use ($object) { unset($object->ro); });
            return [$raw, $unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-private-set-hook-unset'] = function () {
    return w13R6Ghost(W13R6PrivateHook::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->p); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-raw-plain-unset'] = function () {
    return w13R6Ghost(W13R6RawPlain::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $property = $reflection->getProperty('q');
            $property->setRawValueWithoutLazyInitialization($object, 11);
            $raw = $property->getRawValue($object);
            $unset = attempt(function () use ($object) { unset($object->q); });
            return [$raw, $unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-dynamic-magic'] = function () {
    return w13R6Ghost(W13R6DynamicMagic::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->missing); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-dynamic-plain'] = function () {
    return w13R6Ghost(W13R6DynamicPlain::class,
        function ($object) { w13R6Event('init'); throw new Exception('init'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->missing); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-readonly-init-unset'] = function () {
    return w13R6Ghost(W13R6ReadonlyInit::class,
        function ($object) { w13R6Event('init'); $object->initializeReadonly(); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { $object->clearReadonly(); });
            return [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-plain-unset'] = function () {
    return w13R6Ghost(W13R6Plain::class,
        function ($object) { w13R6Event('init'); $object->q = 11; },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->q); });
            $isset = attempt(function () use ($object) { return isset($object->q); });
            $read = attempt(function () use ($object) { return $object->q; });
            return [$unset, $isset, $read, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-init-throw'] = function () {
    return w13R6Ghost(W13R6ThrowPlain::class,
        function ($object) { w13R6Event('init'); throw new Exception('boom'); },
        function ($object, ReflectionClass $reflection) {
            $unset = attempt(function () use ($object) { unset($object->q); });
            $afterUnset = [$unset, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
            $read = attempt(function () use ($object) { return $object->q; });
            return [...$afterUnset, $read, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
        });
};
$w13R6Tests['r6-lazy-magic-asym'] = function () {
    $GLOBALS['w13_r6_events'] = [];
    $object = new W13R6MagicAsym;
    $unset = attempt(function () use ($object) { unset($object->p); });
    return [$unset, $GLOBALS['w13_r6_events']];
};
$w13R6Tests['r6-lazy-recursive-unset'] = function () {
    $GLOBALS['w13_r6_events'] = [];
    $object = new W13R6RecurChild;
    $clear = attempt(function () use ($object) { $object->clear(); });
    $unset = attempt(function () use ($object) { unset($object->p); });
    return [$clear, $unset, $GLOBALS['w13_r6_events']];
};
$w13R6Tests['r6-lazy-readonly-clone'] = function () {
    $GLOBALS['w13_r6_events'] = [];
    $original = new W13R6RoClone(1);
    $clone = clone $original;
    $value = (new ReflectionProperty(W13R6RoClone::class, 'ro'))->getValue($clone);
    $afterClone = $GLOBALS['w13_r6_events'];
    $unset = attempt(function () use ($clone) { unset($clone->ro); });
    return [$value, $afterClone, $unset, $GLOBALS['w13_r6_events']];
};
$w13R6Tests['r6-lazy-readonly-cleared-magic'] = function () {
    return w13R6Ghost(W13R6RoMagic::class,
        function ($object) { w13R6Event('init'); },
        function ($object, ReflectionClass $reflection) {
            $clear = attempt(function () use ($object) { $object->clear(); });
            $afterClear = [$clear, $GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
            $unset = attempt(function () use ($object) { unset($object->ro); });
            return [...$afterClear, $unset];
        });
};
$tests = $tests ?? [];
$tests += $w13R6Tests;
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R6Tests as $name => $test) {
        $diagnostics = [];
        set_error_handler(function ($level, $message) use (&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        $result = attempt($test);
        restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
