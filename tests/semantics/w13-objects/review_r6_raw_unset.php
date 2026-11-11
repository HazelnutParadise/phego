<?php
// REQ-018/020; T-035/036/039/040. A raw slot remains non-lazy after unset.
class W13R6RawFollowup { public int $q; public int $spare; }
class W13R6RawFollowupMagic extends W13R6RawFollowup {
    public function __get($name): mixed { w13R6Event('get:'.$name); return 23; }
    public function __isset($name): bool { w13R6Event('isset:'.$name); return true; }
    public function __set($name, $value): void { w13R6Event('set:'.$name.':'.$value); }
}
class W13R6RawFollowupMagicThrow extends W13R6RawFollowupMagic {
    public function __set($name, $value): void { w13R6Event('set:'.$name.':'.$value); throw new Exception('set'); }
}
class W13R6RawFollowupMagicReentry extends W13R6RawFollowupMagic {
    public function __set($name, $value): void { w13R6Event('set:'.$name.':'.$value); $this->$name = $value; }
}
foreach (['followup' => W13R6RawFollowup::class, 'magic' => W13R6RawFollowupMagic::class,
          'magic-throw' => W13R6RawFollowupMagicThrow::class, 'magic-reentry' => W13R6RawFollowupMagicReentry::class] as $mode => $class) {
    $tests['r6-lazy-raw-unset-'.$mode] = function() use($mode, $class) {
        $magic = $mode !== 'followup';
        return w13R6Ghost($class,
            function($object) { w13R6Event('init'); throw new Exception('init'); },
            function($object, ReflectionClass $reflection) use($magic) {
                $reflection->getProperty('q')->setRawValueWithoutLazyInitialization($object, 11);
                $result = [attempt(function() use($object) { unset($object->q); }),
                    attempt(fn() => $object->q), attempt(fn() => isset($object->q))];
                if (!$magic) $result[] = attempt(function() use($object) { $alias =& $object->q; unset($alias); });
                $result[] = attempt(function() use($object) { $object->q = 12; });
                $result[] = attempt(fn() => $object->q);
                $result[] = [$GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object)];
                $result[] = attempt(fn() => $object->spare);
                $result[] = [$GLOBALS['w13_r6_calls'], $reflection->isUninitializedLazyObject($object), attempt(fn() => $object->q)];
                return $result;
            });
    };
}
