<?php
// closure_scope_private: the class where a closure is written decides private access
class A {
    private $x;

    public function __construct($x) {
        $this->x = $x;
    }
    public function peekOther(A $other) {
        return function () use ($other) {
            return "other.x=" . $other->x;
        };
    }
    public function setOther(A $other, $v) {
        return function () use ($other, $v) {
            $other->x = $v;
            return "set " . $other->x;
        };
    }
    public function makeIssetCheck() {
        return function () {
            return isset($this->x) ? "x set" : "x unset";
        };
    }
}

class B extends A {
    public function makeIssetCheck2() {
        return function () {
            return isset($this->x) ? "b-sees-x" : "b-no-x";
        };
    }
}

$a1 = new A("one");
$a2 = new A("two");
$peek = $a1->peekOther($a2);
echo "peek_other: ", $peek(), "\n";
echo "a_closure_sets: ", $a1->setOther($a2, "changed")(), "\n";
echo "peek_after_set: ", $a1->peekOther($a2)(), "\n";

$b = new B("bee");
echo "a_method_on_b: ", $b->makeIssetCheck()(), "\n";
echo "b_closure_sees: ", $b->makeIssetCheck2()(), "\n";
echo "a_peek_b_object: ", $a1->peekOther($b)(), "\n";
echo "a1_check: ", $a1->makeIssetCheck()(), "\n";
