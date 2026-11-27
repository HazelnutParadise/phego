<?php
// throw_propagation_kinds: one exception propagates out of each kind of callable and is caught at the top with a marker
class Holder {
    public function method() {
        throw new Exception("method");
    }
    public static function smethod() {
        throw new Exception("static method");
    }
    public function __invoke() {
        throw new Exception("invoke");
    }
}

function plain_fn() {
    throw new Exception("function");
}

$closure = function () {
    throw new Exception("closure");
};
$arrow = fn() => throw new Exception("arrow");
$obj = new Holder();
$name = 'plain_fn';
$pair = [$obj, 'method'];
$first_class = plain_fn(...);

echo "marker: function\n";
try {
    plain_fn();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: method\n";
try {
    $obj->method();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: static method\n";
try {
    Holder::smethod();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: closure\n";
try {
    $closure();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: arrow\n";
try {
    $arrow();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: invokable object\n";
try {
    $obj();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: callable string\n";
try {
    $name();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: callable array\n";
try {
    $pair();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

echo "marker: first-class callable\n";
try {
    $first_class();
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}
echo "end\n";
