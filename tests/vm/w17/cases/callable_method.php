<?php
class A {
    public $pub = "pub";
    private $tag = "A-tag";

    private function secret($x) {
        return $this->tag . ":" . $x;
    }

    public function m($x) {
        return "m:" . $x . ":" . $this->pub;
    }

    public function viaThis() {
        $f = $this->secret(...);
        return $f("inside");
    }

    public function makePrivate() {
        return $this->secret(...);
    }

    public function makeM() {
        return $this->m(...);
    }
}

$a = new A;
$m = $a->m(...);
echo "method_closure: ", $m("q"), "\n";
$a->pub = "changed";
echo "sees_new_property: ", $m("r"), "\n";
echo "inside_class_private: ", $a->viaThis(), "\n";

$p = $a->makePrivate();
echo "private_closure_outside: ", $p("outside"), "\n";

$b = new A;
$b->pub = "b-pub";
$mb = $b->m(...);
echo "other_instance: ", $mb("s"), "\n";
echo "first_still_a: ", $m("t"), "\n";

$name = "m";
$dyn = $a->$name(...);
echo "dynamic_name: ", $dyn("u"), "\n";

$mm = $a->makeM();
echo "made_inside_method: ", $mm("v"), "\n";
echo "is_closure: ", $dyn instanceof Closure ? "yes" : "no", "\n";
