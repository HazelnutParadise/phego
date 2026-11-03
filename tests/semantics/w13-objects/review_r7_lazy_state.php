<?php
// REQ-018; T-035/036. Raw slots keep their per-property non-lazy state.
class W13R7LazyState { public int $x = 1; public int $y = 2; }
class W13R7RawParent {
    private int $x = 11;
    public function parentX(): int { return $this->x; }
}
class W13R7RawChildPublic extends W13R7RawParent { public int $x = 22; }
class W13R7RawChildPrivate extends W13R7RawParent { private int $x = 22; }
foreach (['public' => W13R7RawChildPublic::class, 'private' => W13R7RawChildPrivate::class] as $visibility => $class) {
    $tests['r7-raw-shadow-'.$visibility] = function() use($class) {
        $calls = 0; $r = new ReflectionClass($class); $x = $r->getProperty('x');
        $object = $r->newLazyGhost(function($object) use(&$calls) { $calls++; });
        $result = [];
        foreach ([41, 42] as $value) {
            $x->setRawValueWithoutLazyInitialization($object, $value);
            $result[] = [$r->isUninitializedLazyObject($object), $x->isLazy($object), $x->getRawValue($object), $calls];
        }
        $result[] = w13R7Attempt(fn() => $object->parentX()); $result[] = $calls;
        return $result;
    };
}
class W13R7RawSingle {
    public int $x = 1;
    public function __destruct() { $GLOBALS['w13_r7_raw_events'][] = 'dtor'; }
}
class W13R7RawInitializerOwner {
    public function __destruct() {
        $object = $GLOBALS['w13_r7_raw_owner'];
        $GLOBALS['w13_r7_raw_events'][] = ['release', (new ReflectionClass($object))->isUninitializedLazyObject($object), $object->x];
    }
}
$tests['r7-raw-last-slot'] = function() {
    $GLOBALS['w13_r7_raw_events'] = [];
    $token = new W13R7RawInitializerOwner;
    $r = new ReflectionClass(W13R7RawSingle::class);
    $object = $r->newLazyGhost(function($object) use($token) { $GLOBALS['w13_r7_raw_events'][] = 'init'; });
    unset($token);
    $GLOBALS['w13_r7_raw_owner'] = $object;
    $before = $r->isUninitializedLazyObject($object);
    $x = $r->getProperty('x');
    $error = w13R7Attempt(fn() => $x->setRawValueWithoutLazyInitialization($object, 42));
    $result = [$before, $error, $r->isUninitializedLazyObject($object), $x->isLazy($object), $object->x, $GLOBALS['w13_r7_raw_events']];
    unset($GLOBALS['w13_r7_raw_owner'], $object);
    return [$result, $GLOBALS['w13_r7_raw_events']];
};
$tests['r7-raw-type-error'] = function() {
    $calls = 0; $r = new ReflectionClass(W13R7LazyState::class);
    $object = $r->newLazyGhost(function($object) use(&$calls) { $calls++; });
    $x = $r->getProperty('x'); $y = $r->getProperty('y');
    $error = w13R7Attempt(fn() => $x->setRawValueWithoutLazyInitialization($object, []));
    return [$error, $r->isUninitializedLazyObject($object), $x->isLazy($object), $y->isLazy($object), $calls];
};
$tests['r7-raw-proxy-failure'] = function() {
    $calls = 0; $r = new ReflectionClass(W13R7LazyState::class);
    $object = $r->newLazyProxy(function($object) use(&$calls) { $calls++; $object->x = 41; throw new Exception('factory'); });
    $x = $r->getProperty('x'); $y = $r->getProperty('y');
    $result = [w13R7Attempt(fn() => $object->y), [$x->isLazy($object), $y->isLazy($object)]];
    $x->setRawValueWithoutLazyInitialization($object, 42);
    $result[] = [$x->isLazy($object), $y->isLazy($object)];
    $y->setRawValueWithoutLazyInitialization($object, 2);
    $result[] = [$r->isUninitializedLazyObject($object), $x->isLazy($object), $y->isLazy($object), $object->x, $object->y, $calls];
    $result[] = w13R7Attempt(function() use($object, $r) {
        $copy = clone $object;
        return [$copy->x, $copy->y, $copy !== $object, $r->isUninitializedLazyObject($copy)];
    });
    $outer = $r->newLazyProxy(fn() => $object);
    $result[] = [w13R7Attempt(fn() => $outer->y), $r->isUninitializedLazyObject($outer)];
    return $result;
};
$tests['r7-raw-running-proxy-failure'] = function() {
    $calls = 0; $r = new ReflectionClass(W13R7LazyState::class);
    $object = $r->newLazyProxy(function($object) use(&$calls, $r) {
        $calls++; $r->getProperty('x')->setRawValueWithoutLazyInitialization($object, 41);
        throw new Exception('factory');
    });
    $x = $r->getProperty('x'); $y = $r->getProperty('y');
    $result = [w13R7Attempt(fn() => $object->y), [$r->isUninitializedLazyObject($object), $x->isLazy($object), $y->isLazy($object), $calls]];
    $y->setRawValueWithoutLazyInitialization($object, 42);
    $result[] = [$r->isUninitializedLazyObject($object), $x->isLazy($object), $y->isLazy($object), $object->x, $object->y, $calls];
    return $result;
};
class W13R7RawString {
    public string $x = 'old';
    public function __destruct() { $GLOBALS['w13_r7_raw_rebind_events'][] = 'dtor:'.$this->x; }
}
class W13R7RawRebindSource {
    public function __construct(private WeakReference $weak) {}
    public function __toString(): string {
        $GLOBALS['w13_r7_raw_alias'] = null;
        $GLOBALS['w13_r7_raw_rebind_events'][] = ['convert', $this->weak->get() !== null];
        return 'raw';
    }
}
$tests['r7-raw-receiver-reference-rebind'] = function() {
    $GLOBALS['w13_r7_raw_rebind_events'] = [];
    $r = new ReflectionClass(W13R7RawString::class);
    $object = $r->newLazyGhost(function($object) {}); $alias =& $object;
    $GLOBALS['w13_r7_raw_alias'] =& $alias;
    $weak = WeakReference::create($object); $source = new W13R7RawRebindSource($weak);
    $error = w13R7Attempt(function() use(&$object, $source, $r) {
        return $r->getProperty('x')->setRawValueWithoutLazyInitialization($object, $source);
    });
    unset($GLOBALS['w13_r7_raw_alias']);
    return [$error, $object, $alias, $GLOBALS['w13_r7_raw_rebind_events'], $weak->get() !== null];
};
foreach ([false, true] as $proxy) {
    $tests['r7-lazy-state-'.($proxy ? 'proxy' : 'ghost')] = function() use($proxy) {
        $r = new ReflectionClass(W13R7LazyState::class);
        $x = $r->getProperty('x'); $y = $r->getProperty('y'); $running = [];
        $init = function($object) use($proxy, $x, $y, &$running) {
            $running = [$x->isLazy($object), $y->isLazy($object)];
            return $proxy ? new W13R7LazyState : null;
        };
        $object = $proxy ? $r->newLazyProxy($init) : $r->newLazyGhost($init);
        $result = [[$x->isLazy($object), $y->isLazy($object)]];
        $x->setRawValueWithoutLazyInitialization($object, 42);
        $result[] = [$x->isLazy($object), $y->isLazy($object)];
        unset($object->x);
        $result[] = [$x->isLazy($object), $y->isLazy($object)];
        $result[] = $object->y;
        $result[] = $running;
        $result[] = [$x->isLazy($object), $y->isLazy($object)];
        return $result;
    };
}
