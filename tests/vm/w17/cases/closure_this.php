<?php
// closure_this: closures made in methods bind $this and the class scope
class Base {
    const TAG = "base-tag";
    private $secret = "s1";
    protected $prot = "p1";

    private function hidden($x) {
        return "hidden:" . $x;
    }
    public static function name() {
        return "Base";
    }
    public function greet() {
        return "base-greet";
    }
    public function makeReader() {
        return function () {
            return $this->secret . "|" . $this->prot;
        };
    }
    public function makeWriter($v) {
        return function ($v2) use ($v) {
            $this->secret = $v . $v2;
            return $this->secret;
        };
    }
    public function makeHidden() {
        return function ($x) {
            return $this->hidden($x);
        };
    }
    public function makeConst() {
        return function () {
            return self::TAG . "/" . static::name() . "/" . static::class;
        };
    }
}

class Child extends Base {
    public static function name() {
        return "Child";
    }
    public function greet() {
        return "child-greet";
    }
    public function viaParent() {
        return function () {
            return "viaParent:" . parent::greet() . "+" . $this->greet() . "+" . $this->prot;
        };
    }
}

$b = new Base();
$reader = $b->makeReader();
echo "reader: ", $reader(), "\n";
$writer = $b->makeWriter("W");
echo "writer: ", $writer("!"), "\n";
echo "reader_after_write: ", $reader(), "\n";
$hid = $b->makeHidden();
echo "private_method: ", $hid("q"), "\n";
echo "self_const_static_base: ", $b->makeConst()(), "\n";

$c = new Child();
echo "child_const_static: ", $c->makeConst()(), "\n";
echo "child_parent_call: ", $c->viaParent()(), "\n";
echo "child_reader: ", $c->makeReader()(), "\n";
echo "child_hidden: ", $c->makeHidden()("z"), "\n";
