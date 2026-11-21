<?php
function f($x) {
    return "f:" . $x;
}

class A {
    public $tag = "tagA";

    public function m($x) {
        return "m:" . $x . "/" . $this->tag;
    }

    public static function sm($x) {
        return "sm:" . $x;
    }

    private function priv($x) {
        return "priv:" . $x;
    }

    public function fromInside() {
        return Closure::fromCallable([$this, 'priv']);
    }
}

class I {
    public function __invoke($x) {
        return "I-invoke:" . $x;
    }
}

class M {
    public function __call($name, $args) {
        return "M-call:" . $name;
    }
}

$cf = Closure::fromCallable('f');
echo "function_name: ", $cf("a"), "\n";

$o = new A;
$cm = Closure::fromCallable([$o, 'm']);
echo "object_array: ", $cm("b"), "\n";

$cs = Closure::fromCallable('A::sm');
echo "static_string: ", $cs("c"), "\n";

$cs2 = Closure::fromCallable(['A', 'sm']);
echo "static_array: ", $cs2("d"), "\n";

$clo = function ($x) {
    return "clo:" . $x;
};
$same = Closure::fromCallable($clo);
echo "closure_identity: ", $same === $clo ? "same" : "different", "\n";

$ci = Closure::fromCallable(new I);
echo "object_with_invoke: ", $ci("e"), "\n";

$cn = Closure::fromCallable([new M, 'nope']);
echo "magic_call_name: ", $cn("f"), "\n";

echo "private_from_inside: ", $o->fromInside()("g"), "\n";
echo "closure_is_closure: ", $cm instanceof Closure ? "yes" : "no", "\n";
