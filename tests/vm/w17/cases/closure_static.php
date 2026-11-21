<?php
// closure_static: static closures have no $this but keep the class scope
class Holder {
    public static $count = 5;
    const LIMIT = 9;
    private $inst = "instance";

    public static function name() {
        return "Holder";
    }
    public function makeStatic() {
        return static function () {
            return isset($this) ? "has this" : "no this";
        };
    }
    public function makeStaticScope() {
        return static function () {
            return self::LIMIT . "/" . static::name() . "/" . self::$count;
        };
    }
    public function makeStaticFn() {
        return static fn() => isset($this) ? "fn has this" : "fn no this";
    }
    public function hasInst() {
        return isset($this->inst) ? "inst set" : "inst unset";
    }
}

$h = new Holder();
$s = $h->makeStatic();
echo "static_closure: ", $s(), "\n";
$sc = $h->makeStaticScope();
echo "static_scope: ", $sc(), "\n";
$sf = $h->makeStaticFn();
echo "static_arrow: ", $sf(), "\n";
echo "instance_still_sees_inst: ", $h->hasInst(), "\n";

$top = static function () {
    return isset($this) ? "top has this" : "top no this";
};
echo "top_static: ", $top(), "\n";
$topFn = static fn($x) => $x * 2;
echo "top_static_fn: ", $topFn(21), "\n";

$setter = static function ($v) {
    Holder::$count = $v;
    return Holder::$count;
};
echo "static_prop_write: ", $setter(11), "\n";
echo "static_prop_read: ", Holder::$count, "\n";

class Registry {
    private static $items = [];
    public static function maker() {
        return static function ($k, $v) {
            self::$items[$k] = $v;
            return self::$items[$k];
        };
    }
}
$put = Registry::maker();
echo "registry_put: ", $put("a", "va"), "\n";
echo "registry_put_again: ", $put("b", "vb"), "\n";
