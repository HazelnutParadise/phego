<?php
// closure_destruct_timing: when captured objects are destroyed
class D {
    public $name;

    public function __construct($name) {
        $this->name = $name;
        echo "construct ", $name, "\n";
    }
    public function __destruct() {
        echo "destruct ", $this->name, "\n";
    }
    public function makeCloseOverThis() {
        return function () {
            return "closure of " . $this->name;
        };
    }
}

function local() {
    $item = new D("local");
    $c = function () use ($item) {
        return 1;
    };
    return $c();
}

echo "step1\n";
$o = new D("a");
$f = function () use ($o) {
    return "f sees " . $o->name;
};
unset($o);
echo "step2 after unset of o\n";
echo $f(), "\n";
unset($f);
echo "step3 after unset of f\n";

$p = new D("p");
$g = function () use ($p) {
    return "g";
};
$p = null;
echo "step4 p nulled, closure still holds it\n";
$g = null;
echo "step5 closure overwritten\n";

$t = new D("t");
$r = (function () use ($t) {
    return "temp result";
})();
echo "step6 result: ", $r, "\n";
unset($t);
echo "step7 after unset t\n";

$m = new D("m");
$mc = $m->makeCloseOverThis();
unset($m);
echo "step8 object kept alive by its closure\n";
echo $mc(), "\n";
unset($mc);
echo "step9 after unset of the bound closure\n";

$v = local();
echo "step10 local result: ", $v, "\n";
echo "step11 end\n";
