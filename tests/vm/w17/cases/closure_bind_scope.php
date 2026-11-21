<?php
class A {
    private $priv = "A-private";
    private static $sp = "A-static-private";
    private const C = "A-const";

    public function makeReader() {
        return function () {
            return $this->priv;
        };
    }
}

class B {
    public $pub = "B-public";
    private $priv = "B-private";
}

$a = new A;
$other = new A;
$b = new B;

$readPriv = function () {
    return $this->priv;
};
$bound = $readPriv->bindTo($a, A::class);
echo "scope_A_read_priv: ", $bound(), "\n";

$scopeFromObj = $readPriv->bindTo($a, $other);
echo "scope_from_object: ", $scopeFromObj(), "\n";

$checkPriv = function () {
    return isset($this->priv) ? "visible" : "hidden";
};
echo "no_scope_isset: ", $checkPriv->bindTo($a)(), "\n";
echo "scope_A_isset: ", $checkPriv->bindTo($a, A::class)(), "\n";

$readB = function () {
    return $this->priv;
};
echo "scope_B_read_priv: ", $readB->bindTo($b, B::class)(), "\n";

$staticRead = function () {
    return self::$sp . "/" . self::C;
};
$sr = $staticRead->bindTo(null, A::class);
echo "null_this_scope_A: ", $sr(), "\n";

$reader = $a->makeReader();
echo "static_keeps_scope: ", $reader->bindTo($other, "static")(), "\n";

$pubRead = function () {
    return $this->pub;
};
echo "public_needs_no_scope: ", $pubRead->bindTo($b)(), "\n";
