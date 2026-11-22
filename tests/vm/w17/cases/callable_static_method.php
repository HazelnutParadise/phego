<?php
class A {
    protected $id = "a1";

    public static function sm($x) {
        return "A.sm:" . $x . "@" . static::class;
    }

    public function inst($x) {
        return "A.inst:" . $x . "@" . $this->id;
    }

    public function makeSelf() {
        return self::sm(...);
    }

    public function makeStatic() {
        return static::sm(...);
    }
}

class B extends A {
    public static function sm($x) {
        return "B.sm:" . $x . "@" . static::class;
    }

    public function inst($x) {
        return "B.inst:" . $x . "@" . $this->id;
    }

    public function callerSelf() {
        return self::sm(...);
    }

    public function callerStatic() {
        return static::sm(...);
    }

    public function callerParent() {
        return parent::sm(...);
    }

    public function callerParentInst() {
        return parent::inst(...);
    }
}

$outsideA = A::sm(...);
echo "outside_A: ", $outsideA("o"), "\n";
$outsideB = B::sm(...);
echo "outside_B: ", $outsideB("o"), "\n";

$a = new A;
echo "made_by_self_on_A: ", $a->makeSelf()("p"), "\n";
echo "made_by_static_on_A: ", $a->makeStatic()("q"), "\n";

$b = new B;
echo "self_in_A_on_B: ", $b->makeSelf()("r"), "\n";
echo "static_in_A_on_B: ", $b->makeStatic()("s"), "\n";
echo "callerSelf_on_B: ", $b->callerSelf()("t"), "\n";
echo "callerStatic_on_B: ", $b->callerStatic()("u"), "\n";
echo "callerParent_on_B: ", $b->callerParent()("v"), "\n";
echo "callerParentInst_on_B: ", $b->callerParentInst()("w"), "\n";
