<?php
class A {
    private $secret = 10;
    public $pub = 1;

    private function hidden($x) {
        return "hidden " . $x;
    }
}

$o = new A;

$f = function ($a, $b = 5) {
    return $this->secret + $a + $b;
};
echo "call_basic: ", $f->call($o, 1, 2), "\n";
echo "call_default: ", $f->call($o, 1), "\n";
echo "call_named: ", $f->call($o, b: 100, a: 1), "\n";

$h = function ($x) {
    return $this->hidden($x) . " / " . $this->pub;
};
echo "call_private_method: ", $h->call($o, "p"), "\n";

$k = function () {
    return isset($this) ? "bound" : "unbound";
};
echo "call_binds_this: ", $k->call($o), "\n";
echo "direct_call_unbound: ", $k(), "\n";

$s = static function () {
    return "static ran";
};
$sr = $s->call($o);
echo "static_call_is_null: ", $sr === null ? "yes" : "no", "\n";

echo "original_after_call: ", $f->call(new A, 0), "\n";
echo "end\n";
