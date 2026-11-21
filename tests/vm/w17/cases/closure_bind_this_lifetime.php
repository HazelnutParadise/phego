<?php
class Holder {
    public $name;

    public function __construct($n) {
        $this->name = $n;
        echo "construct ", $this->name, "\n";
    }

    public function __destruct() {
        echo "destruct ", $this->name, "\n";
    }
}

$h = new Holder("h1");
$f = function () {
    return "name=" . $this->name;
};
$bound = $f->bindTo($h);
unset($h);
echo "after_unset_h: ", $bound(), "\n";
echo "unsetting_bound\n";
unset($bound);
echo "after_unset_bound\n";

$t = Closure::bind($f, new Holder("tmp"));
echo "temporary_bound\n";
echo "temporary_call: ", $t(), "\n";
unset($t);
echo "after_unset_temporary\n";

$h2 = new Holder("h2");
$keep = Closure::bind($f, $h2);
$h2 = null;
echo "h2_kept_by_closure: ", $keep(), "\n";
$keep = null;
echo "end\n";
