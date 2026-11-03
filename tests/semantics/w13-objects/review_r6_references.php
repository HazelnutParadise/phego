<?php
// REQ-018/020; T-035/036/039/040. Typed property references reaching a by-reference binder.
function w13R6Attempt(callable $f): mixed {
    try { return $f(); } catch (Throwable $problem) { return [$problem::class, $problem->getMessage()]; }
}
class W13R6Stringable {
    public function __toString(): string { $GLOBALS['r6_events'][] = 'convert'; return 'text'; }
}
class W13R6Invokable {
    public function __invoke(): string { $GLOBALS['r6_events'][] = 'invoke'; return 'invoked'; }
    public function method(): string { $GLOBALS['r6_events'][] = 'method'; return 'method'; }
    public static function staticMethod(): string { $GLOBALS['r6_events'][] = 'static'; return 'static'; }
}
class W13R6Source {
    public int|string $unionInt = 1;
    public int $intProp = 1;
    public string|object $objUnion;
    public mixed $mixed;
    public object $obj;
    public array $arr = [];
    public array|string $staticStore = '';
    public function __construct() {
        $this->objUnion = new W13R6Stringable;
        $this->mixed = new W13R6Stringable;
        $this->obj = new W13R6Stringable;
    }
}
class W13R6Sink {
    public function takeString(string &$v): string { $GLOBALS['r6_events'][] = 'body:string:'.get_debug_type($v); return 'ok'; }
    public function takeFloat(float &$v): string { $GLOBALS['r6_events'][] = 'body:float:'.get_debug_type($v); return 'ok'; }
    public function takeObject(object &$v): string { $GLOBALS['r6_events'][] = 'body:object:'.get_debug_type($v); return 'ok'; }
    public function takeInt(int &$v): string { $GLOBALS['r6_events'][] = 'body:int:'.get_debug_type($v); $v = $v + 1; return 'ok'; }
    public function takeCallable(callable &$v): string { $GLOBALS['r6_events'][] = 'body:callable:'.get_debug_type($v); return 'ok'; }
    public function takeVariadic(string &...$v): string { $GLOBALS['r6_events'][] = 'body:variadic:'.count($v); return 'ok'; }
    public function takePrefix(string &$prefix, string &...$v): string { $GLOBALS['r6_events'][] = 'body:prefix:'.count($v); return 'ok'; }
    public function takeNamed(string &$first, string &$second): string { $GLOBALS['r6_events'][] = 'body:named'; return 'ok'; }
}
$tests = $tests ?? [];
$tests['r6-param-direct-union-string'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeString($source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-direct-int-float'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeFloat($source->intProp); });
    return [$result, $GLOBALS['r6_events'], $source->intProp];
};
$tests['r6-param-direct-object-stringable'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeString($source->objUnion); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->objUnion)];
};
$tests['r6-param-direct-mixed-stringable'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeString($source->mixed); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->mixed)];
};
$tests['r6-param-direct-mixed-int'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $source->mixed = 1; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeString($source->mixed); });
    return [$result, $GLOBALS['r6_events'], $source->mixed];
};
$tests['r6-param-direct-union-int'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeInt($source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-direct-object-union'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeObject($source->objUnion); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->objUnion)];
};
$tests['r6-param-ordinary-int-string'] = function() {
    $GLOBALS['r6_events'] = []; $value = 1; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, &$value) { return $sink->takeString($value); });
    return [$result, $GLOBALS['r6_events'], $value];
};
$tests['r6-param-untyped-property'] = function() {
    $GLOBALS['r6_events'] = []; $source = new stdClass; $source->value = 1; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeString($source->value); });
    return [$result, $GLOBALS['r6_events'], $source->value];
};
$tests['r6-param-ordinary-string-int'] = function() {
    $GLOBALS['r6_events'] = []; $value = '7'; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, &$value) { return $sink->takeInt($value); });
    return [$result, $GLOBALS['r6_events'], $value];
};
$tests['r6-param-named-union-string'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $second = 'x';
    $result = w13R6Attempt(function() use ($sink, $source, &$second) { return $sink->takeNamed(second: $second, first: $source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt, $second];
};
$tests['r6-param-variadic-union-string'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeVariadic($source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-variadic-named'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeVariadic(b: 'x', a: $source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-reflect-union-string'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $method = new ReflectionMethod(W13R6Sink::class, 'takeString'); $args = [&$source->unionInt];
    $result = w13R6Attempt(function() use ($method, $sink, $args) { return $method->invokeArgs($sink, $args); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-reflect-ordinary'] = function() {
    $GLOBALS['r6_events'] = []; $value = 1; $sink = new W13R6Sink;
    $method = new ReflectionMethod(W13R6Sink::class, 'takeString'); $args = [&$value];
    $result = w13R6Attempt(function() use ($method, $sink, $args) { return $method->invokeArgs($sink, $args); });
    return [$result, $GLOBALS['r6_events'], $value];
};
$tests['r6-param-callable-invoke'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $source->mixed = new W13R6Invokable; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeCallable($source->mixed); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->mixed)];
};
$tests['r6-param-callable-array'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $source->arr = [new W13R6Invokable, 'method']; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeCallable($source->arr); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->arr)];
};
$tests['r6-param-callable-static'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $source->staticStore = 'W13R6Invokable::staticMethod'; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeCallable($source->staticStore); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->staticStore)];
};
$tests['r6-param-callable-reject'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeCallable($source->obj); });
    return [$result, $GLOBALS['r6_events'], get_debug_type($source->obj)];
};
$tests['r6-param-post-failure'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink, $source) { return $sink->takeString($source->intProp); });
    $write = w13R6Attempt(function() use ($source) { $source->intProp = []; return 'write'; });
    $recover = w13R6Attempt(function() use ($source) { $source->intProp = 2; return 'write'; });
    return [$result, $write, $recover, $source->intProp];
};

$tests['r6-param-variadic-second'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $first = 'ok';
    $result = w13R6Attempt(function() use ($sink, $source, &$first) { return $sink->takeVariadic($first, $source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-variadic-named-second'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $first = 'ok';
    $result = w13R6Attempt(function() use ($sink, $source, &$first) { return $sink->takeVariadic(first: $first, second: $source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};

$tests['r6-param-variadic-fixedprefix'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $prefix = 'ok';
    $result = w13R6Attempt(function() use ($sink, $source, &$prefix) { return $sink->takePrefix($prefix, $source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-variadic-pos-named'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $first = 'ok';
    $result = w13R6Attempt(function() use ($sink, $source, &$first) { return $sink->takeVariadic($first, second: $source->unionInt); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-literal'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink;
    $result = w13R6Attempt(function() use ($sink) { return $sink->takeString('literal'); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-literal-pos-second'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $first = 'ok';
    $result = w13R6Attempt(function() use ($sink, &$first) { return $sink->takeVariadic($first, 'literal'); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
$tests['r6-param-literal-named-second'] = function() {
    $GLOBALS['r6_events'] = []; $source = new W13R6Source; $sink = new W13R6Sink; $first = 'ok';
    $result = w13R6Attempt(function() use ($sink, &$first) { return $sink->takeVariadic(first: $first, second: 'literal'); });
    return [$result, $GLOBALS['r6_events'], $source->unionInt];
};
