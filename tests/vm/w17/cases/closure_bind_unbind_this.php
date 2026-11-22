<?php
class A {
    public $v = "A-val";

    public function usesThis() {
        return function () {
            return $this->v;
        };
    }

    public function noThis() {
        return function () {
            return "no this here";
        };
    }

    public function plain() {
        return "plain";
    }
}

$a = new A;

$c1 = $a->usesThis();
$unb = $c1->bindTo(null);
echo "unbind_using_this_is_null: ", $unb === null ? "yes" : "no", "\n";
echo "still_bound_after_failed_unbind: ", $c1(), "\n";

$c2 = $a->noThis();
$unb2 = $c2->bindTo(null);
echo "unbind_no_this_is_null: ", $unb2 === null ? "yes" : "no", "\n";
echo "call_unbound_no_this: ", $unb2(), "\n";

$m = $a->plain(...);
$unb3 = $m->bindTo(null);
echo "method_closure_unbind_is_null: ", $unb3 === null ? "yes" : "no", "\n";
echo "method_closure_still_works: ", $m(), "\n";
echo "end\n";
