<?php
// Latest PR #73 value and object review reproductions. Keep these functions in
// weak mode: union coercion depends on the declaring file's strictness.
class W13R2VStringReference {
    public string $text = 'value';
    public function &__toString(): string { return $this->text; }
    public function __destruct() { $GLOBALS['events'][] = 'wrapper'; }
}

class W13R2VTypedCompare {
    public int $a;
    public int $b;
}

#[AllowDynamicProperties]
class W13R2VTypedMaterialized {
    public int $a;
    public int $b;
}

class W13R2VUnionIntString {
    public int|string $value;
    public function accept(int|string $value): mixed { return $value; }
    public function produce(mixed $value): int|string { return $value; }
}

class W13R2VUnionIntBool {
    public int|bool $value;
    public function accept(int|bool $value): mixed { return $value; }
    public function produce(mixed $value): int|bool { return $value; }
}

class W13R2VMagicDuplicate {
    public function __call($name, $arguments) {
        $GLOBALS['events'][] = 'magic';
        return [$name, $arguments];
    }
}

class W13R2VMagicDuplicateInaccessible {
    private function hidden($first, $second) { $GLOBALS['events'][] = 'hidden'; }
    public function __call($name, $arguments) {
        $GLOBALS['events'][] = 'magic';
        return [$name, $arguments];
    }
}

class W13R2VClonePrivate {
    private function __clone() { $GLOBALS['events'][] = 'clone'; }
    public function duplicate() { return clone $this; }
    public function __destruct() { $GLOBALS['events'][] = 'destructor'; }
}

class W13R2VCloneProtected {
    protected function __clone() { $GLOBALS['events'][] = 'clone'; }
    public function __destruct() { $GLOBALS['events'][] = 'destructor'; }
}

$tests['r2-values-string-reference-concat'] = function() {
    $GLOBALS['events'] = [];
    $o = new W13R2VStringReference;
    $text = 'prefix:'.$o;
    $before = $GLOBALS['events'];
    unset($o);
    return [$text, $before, $GLOBALS['events']];
};

$tests['r2-values-typed-compare-primary'] = function() {
    $left = new W13R2VTypedCompare;
    $left->a = 1;
    $right = new W13R2VTypedCompare;
    $right->a = 0;
    $right->b = 0;
    return $left <=> $right;
};

$tests['r2-values-typed-compare-reverse'] = function() {
    $left = new W13R2VTypedCompare;
    $left->a = 1;
    $right = new W13R2VTypedCompare;
    $right->a = 0;
    $right->b = 0;
    return $right <=> $left;
};

$tests['r2-values-typed-compare-prefix'] = function() {
    $left = new W13R2VTypedCompare;
    $left->a = 3;
    $left->b = 1;
    $right = new W13R2VTypedCompare;
    $right->a = 3;
    $right->b = 2;
    return $left <=> $right;
};

$tests['r2-values-typed-compare-uninitialized'] = function() {
    $left = new W13R2VTypedCompare;
    $left->a = 3;
    $right = new W13R2VTypedCompare;
    $right->a = 3;
    $right->b = 2;
    return [
        $left <=> $right,
        $right <=> $left,
        get_object_vars($left),
        (array)$left,
        get_object_vars($right),
        (array)$right,
    ];
};

$tests['r2-values-typed-materialized-table'] = function() {
    $left = new W13R2VTypedMaterialized;
    $left->a = 1;
    $right = new W13R2VTypedMaterialized;
    $right->a = 0;
    $right->b = 0;
    $before = $left <=> $right;
    $leftVars = get_object_vars($left);
    $leftCast = (array)$left;
    $afterMaterialize = $left <=> $right;
    $left->extra = 7;
    $afterAdd = $left <=> $right;
    unset($left->extra);
    $afterRemove = $left <=> $right;
    return [
        $before,
        $leftVars,
        $leftCast,
        $afterMaterialize,
        $afterAdd,
        $afterRemove,
        get_object_vars($left),
        (array)$left,
    ];
};

$tests['r2-values-typed-materialized-prefix'] = function() {
    $left = new W13R2VTypedMaterialized;
    $left->a = 3;
    $right = new W13R2VTypedMaterialized;
    $right->a = 3;
    $right->b = 2;
    $before = $left <=> $right;
    get_object_vars($left);
    $afterGetVars = $left <=> $right;
    $left->extra = 7;
    $right->extra = 7;
    $afterAdd = $left <=> $right;
    unset($left->extra, $right->extra);
    $afterRemove = $left <=> $right;
    return [$before, $afterGetVars, $afterAdd, $afterRemove];
};

$tests['r2-values-materialized-clone'] = function() {
    $left=new W13R2VTypedMaterialized;$left->a=3;
    $right=new W13R2VTypedMaterialized;$right->a=3;$right->b=2;
    get_object_vars($left);
    $copy=clone $left;
    $first=$copy <=> $right;
    $left->extra=7;unset($left->extra);
    $copy=clone $left;
    return [$first,$copy <=> $right];
};

$w13R2Inputs = [
    'huge' => 1e30,
    'nan' => NAN,
    'inf' => INF,
    'numeric-string' => '1e30',
];
$w13R2UnionClasses = [
    'intstring' => W13R2VUnionIntString::class,
    'intbool' => W13R2VUnionIntBool::class,
];
foreach ($w13R2UnionClasses as $typeName => $className) {
    foreach ($w13R2Inputs as $inputName => $input) {
        foreach (['property', 'argument', 'return'] as $where) {
            $testName = 'r2-values-union-'.$where.'-'.$typeName.'-'.$inputName;
            $tests[$testName] = function() use ($className, $input, $where) {
                $o = new $className;
                if ($where === 'property') {
                    return attempt(function() use ($o, $input) {
                        $o->value = $input;
                        return $o->value;
                    });
                }
                if ($where === 'argument') {
                    return attempt(fn() => $o->accept($input));
                }
                return attempt(fn() => $o->produce($input));
            };
        }
    }
}

$tests['r2-values-magic-duplicate-missing'] = function() {
    $GLOBALS['events'] = [];
    $o = new W13R2VMagicDuplicate;
    $first = ['duplicate' => 1];
    $second = ['duplicate' => 2];
    $result = attempt(fn() => $o->missing(...$first, ...$second));
    return [$result, $GLOBALS['events']];
};

$tests['r2-values-magic-duplicate-inaccessible'] = function() {
    $GLOBALS['events'] = [];
    $o = new W13R2VMagicDuplicateInaccessible;
    $first = ['duplicate' => 1];
    $second = ['duplicate' => 2];
    $result = attempt(fn() => $o->hidden(...$first, ...$second));
    return [$result, $GLOBALS['events']];
};

foreach (['private' => W13R2VClonePrivate::class, 'protected' => W13R2VCloneProtected::class] as $visibility => $className) {
    foreach (['ordinary', 'ghost', 'proxy'] as $kind) {
        $testName = 'r2-values-clone-'.$kind.'-'.$visibility;
        $tests[$testName] = function() use ($className, $kind) {
            $GLOBALS['events'] = [];
            $reflection = new ReflectionClass($className);
            if ($kind === 'ghost') {
                $o = $reflection->newLazyGhost(function() {
                    $GLOBALS['events'][] = 'initializer';
                });
            } elseif ($kind === 'proxy') {
                $o = $reflection->newLazyProxy(function() use ($className) {
                    $GLOBALS['events'][] = 'initializer';
                    return new $className;
                });
            } else {
                $o = new $className;
            }
            $copy = null;
            $result = attempt(function() use ($o, &$copy) {
                $copy = clone $o;
                return $copy;
            });
            $afterClone = $GLOBALS['events'];
            unset($copy, $o);
            return [$result, $afterClone, $GLOBALS['events']];
        };
    }
}

$tests['r2-values-clone-in-scope'] = function() {
    $GLOBALS['events'] = [];
    $o = new W13R2VClonePrivate;
    $copy = $o->duplicate();
    $afterClone = $GLOBALS['events'];
    unset($copy, $o);
    return [$afterClone, $GLOBALS['events']];
};
