<?php
// REQ-012/018; T-023/024/035/036. Property accessibility, hook initialization and callable validation.
function w13R4Attempt(callable $f): mixed {
    try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
}

class W13R4VirtualWriteOnly {
    public string $x { set {} }
}
class W13R4SelfIsset {
    public bool $inside = false;
    public int $sets = 0;
    public string $x { set { $this->sets++; $this->inside = isset($this->x); } }
}
class W13R4BackedWriteOnly {
    public ?int $x { set { $this->x = $value; } }
}
class W13R4BackedDefault {
    public ?int $x = 5 { set { $this->x = $value; } }
}
class W13R4Getter {
    public ?int $x { get { $GLOBALS['w13r4_events'][] = 'get'; return $GLOBALS['w13r4_value']; } }
}
class W13R4GetterThrow {
    public int $x { get { $GLOBALS['w13r4_events'][] = 'get'; throw new RuntimeException('getter'); } }
}
class W13R4MagicControl {
    public function __isset(string $name): bool { $GLOBALS['w13r4_events'][] = 'isset:' . $name; return true; }
    public function __get(string $name): mixed { $GLOBALS['w13r4_events'][] = 'get:' . $name; return 9; }
}
class W13R4Lazy {
    public int $stored = 0;
    public int $virtual { get { $GLOBALS['w13r4_events'][] = 'get'; return $this->stored; } }
}
class W13R4MagicCallable {
    public function __call(string $name, array $args): string { return 'magic'; }
}
class W13R4Callable {
    public function __invoke(): string { return 'invoke'; }
}
class W13R4CallableSink {
    public function acceptWeak(callable $value): string { return 'body'; }
    public function acceptStrict(callable $value): string { return 'body'; }
    public function returnWeak(): callable { return $GLOBALS['w13r4_callable']; }
    public function badReturnWeak(): callable { return $GLOBALS['w13r4_bad_callable']; }
}

require __DIR__.'/review_r4_strict.php';

$tests['r4-access-virtual-writeonly'] = function() {
    $o = new W13R4VirtualWriteOnly;
    $set = w13R4Attempt(function() use ($o) { $o->x = 'value'; return 'set'; });
    return [$set, w13R4Attempt(fn() => isset($o->x)), w13R4Attempt(fn() => $o->x)];
};
$tests['r4-access-self-isset'] = function() {
    $o = new W13R4SelfIsset;
    $o->x = 'value';
    return [$o->sets, $o->inside, isset($o->x)];
};
$tests['r4-access-backed-uninitialized'] = function() {
    $o = new W13R4BackedWriteOnly;
    return [isset($o->x), w13R4Attempt(fn() => $o->x)];
};
$tests['r4-access-backed-initialized'] = function() {
    $o = new W13R4BackedWriteOnly;
    $o->x = 7;
    return [isset($o->x), $o->x];
};
$tests['r4-access-backed-default'] = function() {
    $o = new W13R4BackedDefault;
    return [isset($o->x), $o->x];
};
$tests['r4-access-getter-null'] = function() {
    $GLOBALS['w13r4_events'] = []; $GLOBALS['w13r4_value'] = null; $o = new W13R4Getter;
    $isset = isset($o->x); $read = $o->x;
    return [$isset, $read, $GLOBALS['w13r4_events']];
};
$tests['r4-access-getter-value'] = function() {
    $GLOBALS['w13r4_events'] = []; $GLOBALS['w13r4_value'] = 4; $o = new W13R4Getter;
    $isset = isset($o->x); $read = $o->x;
    return [$isset, $read, $GLOBALS['w13r4_events']];
};
$tests['r4-access-getter-throw'] = function() {
    $GLOBALS['w13r4_events'] = []; $o = new W13R4GetterThrow;
    return [w13R4Attempt(fn() => isset($o->x)), w13R4Attempt(fn() => $o->x), $GLOBALS['w13r4_events']];
};
$tests['r4-access-magic-control'] = function() {
    $GLOBALS['w13r4_events'] = []; $o = new W13R4MagicControl;
    $isset = isset($o->x); $read = $o->x;
    return [$isset, $read, $GLOBALS['w13r4_events']];
};
$tests['r4-access-lazy-ghost'] = function() {
    $GLOBALS['w13r4_events'] = []; $r = new ReflectionClass(W13R4Lazy::class);
    $o = $r->newLazyGhost(function($o) { $GLOBALS['w13r4_events'][] = 'initialize'; $o->stored = 8; });
    $before = $r->isUninitializedLazyObject($o); $isset = isset($o->virtual);
    return [$before, $isset, $r->isUninitializedLazyObject($o), $GLOBALS['w13r4_events']];
};
$tests['r4-access-lazy-proxy'] = function() {
    $GLOBALS['w13r4_events'] = []; $r = new ReflectionClass(W13R4Lazy::class); $target = new W13R4Lazy; $target->stored = 6;
    $o = $r->newLazyProxy(function() use ($target) { $GLOBALS['w13r4_events'][] = 'initialize'; return $target; });
    $before = $r->getLazyInitializer($o) !== null; $isset = isset($o->virtual);
    return [$before, $isset, $r->getLazyInitializer($o) === null, $GLOBALS['w13r4_events']];
};
$tests['r4-callable-magic-bare'] = function() {
    $o = new W13R4MagicCallable; $sink = new W13R4StrictCallableSink;
    $invoke = new ReflectionMethod($sink, 'acceptWeak');
    return [is_callable($o), w13R4Attempt(fn() => $invoke->invoke($sink, $o))];
};
$tests['r4-callable-magic-array'] = function() {
    $o = new W13R4MagicCallable; $sink = new W13R4StrictCallableSink; $callable = [$o, 'missing'];
    $arg = (new ReflectionMethod($sink, 'acceptWeak'))->invoke($sink, $callable);
    $GLOBALS['w13r4_callable'] = $callable;
    $ret = (new ReflectionMethod($sink, 'returnWeak'))->invoke($sink);
    $result = [is_callable($callable), $arg, is_callable($ret)];
    unset($GLOBALS['w13r4_callable']);
    return $result;
};
$tests['r4-callable-weak-strict-args'] = function() {
    $sink = new W13R4StrictCallableSink; $good = new W13R4Callable; $bad = new W13R4MagicCallable;
    $weak = new ReflectionMethod($sink, 'acceptWeak');
    return [$weak->invoke($sink, $good), w13R4StrictAccept($sink, $good), w13R4Attempt(fn() => $weak->invoke($sink, $bad)), w13R4Attempt(fn() => w13R4StrictAccept($sink, $bad))];
};
$tests['r4-callable-weak-strict-returns'] = function() {
    $sink = new W13R4StrictCallableSink; $GLOBALS['w13r4_callable'] = new W13R4Callable; $GLOBALS['w13r4_bad_callable'] = new W13R4MagicCallable;
    $weak = (new ReflectionMethod($sink, 'returnWeak'))->invoke($sink); $strict = (new ReflectionMethod($sink, 'returnStrict'))->invoke($sink);
    $badWeak = w13R4Attempt(fn() => (new ReflectionMethod($sink, 'badReturnWeak'))->invoke($sink));
    $badStrict = w13R4Attempt(fn() => (new ReflectionMethod($sink, 'badReturnStrict'))->invoke($sink));
    $result = [is_callable($weak), is_callable($strict), $badWeak, $badStrict];
    unset($GLOBALS['w13r4_callable'], $GLOBALS['w13r4_bad_callable']);
    return $result;
};
$tests['r4-callable-private-invoke'] = function() {
    set_error_handler(function() { return true; });
    eval('class W13R4PrivateInvoke { private function __invoke(): string { return "private"; } }');
    eval('class W13R4ProtectedInvoke { protected function __invoke(): string { return "protected"; } }');
    restore_error_handler();
    $private = new W13R4PrivateInvoke; $protected = new W13R4ProtectedInvoke;
    return [[is_callable($private), w13R4Attempt(fn() => $private())], [is_callable($protected), w13R4Attempt(fn() => $protected())]];
};
$tests['r4-callable-inherited-invoke'] = function() {
    set_error_handler(function() { return true; });
    eval('class W13R4ParentInvoke { private function __invoke(): string { return "parent"; } }');
    eval('class W13R4ChildInvoke extends W13R4ParentInvoke { public function __invoke(): string { return "child"; } }');
    restore_error_handler();
    $o = new W13R4ChildInvoke;
    return [is_callable($o), $o()];
};
