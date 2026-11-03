<?php
// PHP 8.5.12 value coercion and Stringable ownership review cases.
// Keep this file weakly typed: argument and return coercion is the subject.
class W13R3ValuePlain {
    public function __toString(): string { return 'text'; }
}
class W13R3ValueStringBase {
    public function __toString(): string { return 'text'; }
}
class W13R3ValueInherited extends W13R3ValueStringBase {}
trait W13R3ValueStringTrait {
    public function __toString(): string { return 'text'; }
}
class W13R3ValueTraitUser { use W13R3ValueStringTrait; }
class W13R3ValueNotStringable {}
class W13R3ValueStringBox { public Stringable $value; }
class W13R3ValueStringArgument {
    public function accept(Stringable $value): bool { return $value instanceof Stringable; }
}
class W13R3ValueStringReturn {
    public function produce(Stringable $value): Stringable { return $value; }
}
class W13R3ValueNegativeBox { public Stringable $value; }
class W13R3ValueNegativeArgument {
    public function accept(Stringable $value): bool { return true; }
}
class W13R3ValueNegativeReturn {
    public function produce(): Stringable { return new W13R3ValueNotStringable; }
}
class W13R3FloatProperty { public string $value; }
class W13R3FloatArgument {
    public function accept(string $value): string { return $value; }
}
class W13R3FloatReturn {
    public function produce(mixed $value): string { return $value; }
}
class W13R3IntStringProperty { public int|string $value; }
class W13R3IntStringArgument {
    public function accept(int|string $value): int|string { return $value; }
}
class W13R3IntStringReturn {
    public function produce(mixed $value): int|string { return $value; }
}

$w13R3StringableClasses = [
    'plain' => W13R3ValuePlain::class,
    'inherited' => W13R3ValueInherited::class,
    'trait' => W13R3ValueTraitUser::class,
];
$w13R3StrictProperty = eval('declare(strict_types=1); return static function($box, $value) { $box->value = $value; return $box->value instanceof Stringable; };');
$w13R3StrictArgument = eval('declare(strict_types=1); return static function($box, $value) { return $box->accept($value); };');
$w13R3StrictReturn = eval('declare(strict_types=1); return static function(Stringable $value): Stringable { return $value; };');
$w13R3WeakReturn = static function(Stringable $value): Stringable { return $value; };
foreach ($w13R3StringableClasses as $family => $className) {
    foreach (['property', 'argument', 'return'] as $path) {
        foreach (['weak', 'strict'] as $mode) {
            $testName = 'r3-value-stringable-'.$family.'-'.$path.'-'.$mode;
            $tests[$testName] = function() use ($className, $path, $mode, $w13R3StrictProperty, $w13R3StrictArgument, $w13R3StrictReturn, $w13R3WeakReturn) {
                $value = new $className;
                if ($path === 'property') {
                    $box = new W13R3ValueStringBox;
                    $ok = $mode === 'strict' ? $w13R3StrictProperty($box, $value) : ($box->value = $value) instanceof Stringable;
                    return (bool)$ok;
                }
                if ($path === 'argument') {
                    $box = new W13R3ValueStringArgument;
                    return $mode === 'strict' ? $w13R3StrictArgument($box, $value) : $box->accept($value);
                }
                $returned = $mode === 'strict' ? $w13R3StrictReturn($value) : $w13R3WeakReturn($value);
                return $returned === $value;
            };
        }
    }
}

foreach (['property', 'argument', 'return'] as $path) {
    $testName = 'r3-value-stringable-negative-'.$path;
    $tests[$testName] = function() use ($path) {
        $value = new W13R3ValueNotStringable;
        if ($path === 'property') {
            $box = new W13R3ValueNegativeBox;
            $result = attempt(function() use ($box, $value) { $box->value = $value; return true; });
            return $result;
        }
        if ($path === 'argument') {
            $r=new ReflectionMethod(W13R3ValueNegativeArgument::class,'accept');
            return attempt(fn() => $r->invoke(new W13R3ValueNegativeArgument,$value));
        }
        $result = attempt(fn() => (new W13R3ValueNegativeReturn)->produce());
        return $result;
    };
}

ini_set('precision', '14');
$w13R3Floats = [
    'precision' => 1.2345678901234567,
    'exponent' => 1.0e20,
    'negative-zero' => -0.0,
    'boundary-fixed' => 99999999999999.0,
    'boundary-exponent' => 1.0e14,
];
foreach ($w13R3Floats as $inputName => $input) {
    foreach (['property', 'argument', 'return'] as $path) {
        $testName = 'r3-value-float-string-'.$path.'-'.$inputName;
        $tests[$testName] = function() use ($input, $path) {
            if ($path === 'property') {
                $box = new W13R3FloatProperty;
                return attempt(function() use ($box, $input) { $box->value = $input; return $box->value; });
            }
            if ($path === 'argument') {
                return attempt(fn() => (new W13R3FloatArgument)->accept($input));
            }
            return attempt(fn() => (new W13R3FloatReturn)->produce($input));
        };
    }
}

$w13R3Overflow = [
    'huge' => 1.0e30,
    'negative-huge' => -1.0e30,
    'nan' => NAN,
    'inf' => INF,
];
foreach ($w13R3Overflow as $inputName => $input) {
    foreach (['property', 'argument', 'return'] as $path) {
        $testName = 'r3-value-intstring-overflow-'.$path.'-'.$inputName;
        $tests[$testName] = function() use ($input, $path) {
            if ($path === 'property') {
                $box = new W13R3IntStringProperty;
                return attempt(function() use ($box, $input) { $box->value = $input; return $box->value; });
            }
            if ($path === 'argument') {
                return attempt(fn() => (new W13R3IntStringArgument)->accept($input));
            }
            return attempt(fn() => (new W13R3IntStringReturn)->produce($input));
        };
    }
}

class W13R3OwnedStringable {
    public function __toString(): string {
        $GLOBALS['events'][] = 'convert';
        unset($GLOBALS['owner']);
        if (($GLOBALS['failure'] ?? '') === 'conversion') { throw new Exception('conversion'); }
        return 'converted';
    }
    public function __destruct() {
        $GLOBALS['events'][] = 'destructor';
        if (($GLOBALS['failure'] ?? '') === 'destructor') { throw new Exception('destructor'); }
    }
}
class W13R3OwnedStringSink {
    public function accept(string $value): string { $GLOBALS['events'][] = 'body'; return $value; }
}
class W13R3OwnedStringDirectReturn {
    public function produce(): string { return $GLOBALS['owner']; }
}
foreach (['', '-frame'] as $frame) {
  foreach([''=>'','-conversion-throw'=>'conversion','-destructor-throw'=>'destructor'] as $suffix=>$failure){
    $tests['r3-value-return-owned-stringable'.$suffix.$frame] = function()use($failure) {
        $GLOBALS['events'] = [];
        $GLOBALS['failure']=$failure;
        $GLOBALS['owner'] = new W13R3OwnedStringable;
        $previous=null;
        try{$returned=(new W13R3OwnedStringDirectReturn)->produce();}
        catch(Throwable $caught){$returned=[$caught::class,$caught->getMessage()];if($prior=$caught->getPrevious())$previous=[$prior::class,$prior->getMessage()];}
        $alive = isset($GLOBALS['owner']);
        $before = $GLOBALS['events'];
        unset($GLOBALS['owner']);
        return [$returned, $alive, $before, $GLOBALS['events'],$previous];
    };
  }
}
foreach (['' => '', '-conversion-throw' => 'conversion', '-destructor-throw' => 'destructor'] as $suffix => $failure) {
    foreach (['', '-frame'] as $frame) {
        $testName = 'r3-value-return-argument-stringable'.$suffix.$frame;
        $tests[$testName] = function() use ($failure) {
            $GLOBALS['events'] = [];
            $GLOBALS['failure'] = $failure;
            $GLOBALS['owner'] = new W13R3OwnedStringable;
            $returned = attempt(fn() => (new W13R3OwnedStringSink)->accept($GLOBALS['owner']));
            $events = $GLOBALS['events'];
            unset($GLOBALS['owner'], $GLOBALS['failure']);
            return [$returned, $events];
        };
    }
}

class W13R3BorrowedGetter {
    public Stringable $value {
        get { return $GLOBALS['owner']; }
    }
}
class W13R3BorrowedStringGetter {
    public string $value {
        get { return $GLOBALS['owner']; }
    }
}
foreach (['stringable' => W13R3BorrowedGetter::class, 'string-conversion' => W13R3BorrowedStringGetter::class] as $kind => $className) {
    $tests['r3-value-hook-borrowed-'.$kind] = function() use ($className, $kind) {
        $GLOBALS['events'] = [];
        $GLOBALS['owner'] = new class {
            public function __toString(): string { $GLOBALS['events'][] = 'convert'; return 'hook-text'; }
            public function __destruct() { $GLOBALS['events'][] = 'destructor'; }
        };
        $value = (new $className)->value;
        $result = $kind === 'stringable' ? $value instanceof Stringable : $value;
        $before = $GLOBALS['events'];
        unset($GLOBALS['owner']);
        $afterOwner = $GLOBALS['events'];
        unset($value);
        return [$result, $before, $afterOwner, $GLOBALS['events']];
    };
}
