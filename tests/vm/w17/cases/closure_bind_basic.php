<?php
class Box {
    public $label = "box";

    public function __construct($v) {
        $this->label = $v;
    }
}

$f = function () {
    return "label=" . $this->label;
};

$o1 = new Box("first");
$o2 = new Box("second");

$b1 = $f->bindTo($o1);
echo "bindTo_first: ", $b1(), "\n";

$b2 = Closure::bind($f, $o2);
echo "static_bind_second: ", $b2(), "\n";

$b3 = $f->bindTo($o1);
$o1->label = "changed";
echo "bound_sees_property_change: ", $b3(), "\n";

$rebound = $b1->bindTo($o2);
echo "rebind_bound_closure: ", $rebound(), "\n";
echo "earlier_binding_kept: ", $b1(), "\n";
echo "original_is_not_bound_copy: ", $f === $b1 ? "same" : "different", "\n";

$plain = function ($x) {
    return "plain:" . $x;
};
$np = $plain->bindTo(null);
echo "bindTo_null_no_this: ", $np("z"), "\n";
$np2 = Closure::bind($plain, null);
echo "static_bind_null_no_this: ", $np2("w"), "\n";
echo "plain_original: ", $plain("v"), "\n";
