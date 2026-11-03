<?php
// REQ-018/020; T-035/036/039/040. Lazy proxy validation and ownership.
class W13ProxyBase {
    public int $value;
    public string $label = 'base';
    public function who(): array { return [get_class($this), static::class]; }
}
class W13ProxyChild extends W13ProxyBase {}
class W13ProxyWrapper extends W13ProxyBase {
    public function who(): array { return ['wrapper', get_class($this), static::class]; }
}
class W13ProxyUnrelated {}
class W13ProxyExtra extends W13ProxyBase { public int $extra; }
class W13ProxyCloneBase {
    public int $value = 1;
    public function __clone() {
        $GLOBALS['proxyEvents'][] = ['clone', get_class($this)];
        $this->value++;
    }
}
class W13ProxyCloneWrapper extends W13ProxyCloneBase {}
class W13ProxyCloneOverride extends W13ProxyCloneBase {
    public function __clone() { parent::__clone(); }
}
class W13ProxyDestructorBase {
    public int $value;
    public function __destruct() {}
}
class W13ProxyDestructorOverride extends W13ProxyDestructorBase {
    public function __destruct() {}
}
class W13ProxyDefaults {
    public int $typed = 7;
    public $untyped = 'plain-default';
}
$tests += [
    'proxy-scalar-factory' => function() {
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => 7);
        return attempt(fn() => $proxy->value);
    },
    'proxy-incompatible-class' => function() {
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => new stdClass);
        return attempt(fn() => $proxy->value);
    },
    'proxy-incompatible-lazy-class-priority' => function() {
        $ghost = (new ReflectionClass(W13ProxyUnrelated::class))->newLazyGhost(function($o) {});
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => $ghost);
        return attempt(fn() => $proxy->value);
    },
    'proxy-real-subclass-rejected' => function() {
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => new W13ProxyChild);
        return attempt(fn() => $proxy->value);
    },
    'proxy-parent-target-compatible' => function() {
        $real = new W13ProxyBase;
        $real->value = 9;
        $proxy = (new ReflectionClass(W13ProxyWrapper::class))->newLazyProxy(fn() => $real);
        return [get_class($proxy), $proxy->value, get_class($real)];
    },
    'proxy-extra-property-rejected' => function() {
        $proxy = (new ReflectionClass(W13ProxyExtra::class))->newLazyProxy(fn() => new W13ProxyBase);
        return attempt(fn() => $proxy->value);
    },
    'proxy-clone-override-rejected' => function() {
        $proxy = (new ReflectionClass(W13ProxyCloneOverride::class))->newLazyProxy(fn() => new W13ProxyCloneBase);
        return attempt(fn() => $proxy->value);
    },
    'proxy-destructor-override-rejected' => function() {
        $proxy = (new ReflectionClass(W13ProxyDestructorOverride::class))->newLazyProxy(fn() => new W13ProxyDestructorBase);
        return attempt(fn() => $proxy->value);
    },
    'proxy-self-target-rejected' => function() {
        $proxy = null;
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(function() use (&$proxy) { return $proxy; });
        return attempt(fn() => $proxy->value);
    },
    'proxy-uninitialized-ghost-target-rejected' => function() {
        $ghost = (new ReflectionClass(W13ProxyBase::class))->newLazyGhost(function($o) { $o->value = 1; });
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => $ghost);
        return attempt(fn() => $proxy->value);
    },
    'proxy-initialized-ghost-target-accepted' => function() {
        $ghost = (new ReflectionClass(W13ProxyBase::class))->newLazyGhost(function($o) { $o->value = 12; });
        $ghost->value;
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => $ghost);
        return [$proxy->value, get_class($proxy), get_class($ghost)];
    },
    'proxy-initialized-proxy-target-rejected' => function() {
        $target = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(function() {
            $real = new W13ProxyBase;
            $real->value = 13;
            return $real;
        });
        $target->value;
        $proxy = (new ReflectionClass(W13ProxyBase::class))->newLazyProxy(fn() => $target);
        return attempt(fn() => $proxy->value);
    },
    'proxy-wrapper-method-this' => function() {
        $calls = 0;
        $real = new W13ProxyBase;
        $real->value = 14;
        $proxy = (new ReflectionClass(W13ProxyWrapper::class))->newLazyProxy(function() use (&$calls, $real) {
            $calls++;
            return $real;
        });
        $property = new ReflectionProperty(W13ProxyBase::class, 'value');
        $before = [$property->isLazy($proxy), $calls];
        $first = $proxy->who();
        $afterMethod = [$property->isLazy($proxy), $calls, get_class($proxy)];
        $value = $proxy->value;
        $second = $proxy->who();
        return [$before, $first, $afterMethod, $value, $second, $calls, get_class($proxy)];
    },
    'proxy-wrapper-clone-backing' => function() {
        $GLOBALS['proxyEvents'] = [];
        $real = new W13ProxyCloneBase;
        $proxy = (new ReflectionClass(W13ProxyCloneWrapper::class))->newLazyProxy(fn() => $real);
        $clone = clone $proxy;
        return [get_class($proxy), get_class($clone), $clone->value, $GLOBALS['proxyEvents']];
    },
    'proxy-factory-failure-property-write' => function() {
        $calls = 0;
        $proxy = null;
        $real = new W13ProxyDefaults;
        $factory = function() use (&$calls, &$proxy, $real) {
            $calls++;
            $proxy->typed = 41;
            $proxy->untyped = 'factory-write';
            if ($calls === 1) { throw new Exception('factory failed'); }
            return $real;
        };
        $proxy = (new ReflectionClass(W13ProxyDefaults::class))->newLazyProxy($factory);
        $typed = new ReflectionProperty(W13ProxyDefaults::class, 'typed');
        $untyped = new ReflectionProperty(W13ProxyDefaults::class, 'untyped');
        $before = [$typed->isLazy($proxy), $untyped->isLazy($proxy)];
        $first = attempt(fn() => $proxy->typed);
        $afterFailure = [$typed->isLazy($proxy), $untyped->isLazy($proxy)];
        $written = [$proxy->typed, $proxy->untyped];
        return [$before, $first, $afterFailure, $written, $calls];
    },
    'proxy-factory-reads-defaults' => function() {
        $proxy = null;
        $reads = [];
        $real = new W13ProxyDefaults;
        $proxy = (new ReflectionClass(W13ProxyDefaults::class))->newLazyProxy(function() use (&$proxy, &$reads, $real) {
            $typed = new ReflectionProperty(W13ProxyDefaults::class, 'typed');
            $untyped = new ReflectionProperty(W13ProxyDefaults::class, 'untyped');
            $reads = [
                [$typed->isLazy($proxy), $typed->isInitialized($proxy), attempt(fn() => $proxy->typed)],
                [$untyped->isLazy($proxy), $untyped->isInitialized($proxy), attempt(fn() => $proxy->untyped)],
            ];
            return $real;
        });
        $value = $proxy->typed;
        return [$reads, $value, $proxy->untyped];
    },
];
