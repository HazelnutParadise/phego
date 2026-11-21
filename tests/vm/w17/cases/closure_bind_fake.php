<?php
class A {
    public function m() {
        return "A::m";
    }
}

class B {
    public function m() {
        return "B::m";
    }
}

function f() {
    return "f";
}

$a = new A;
$a2 = new A;

$fk = f(...);
$r1 = $fk->bindTo(null, A::class);
echo "function_rebind_scope_is_null: ", $r1 === null ? "yes" : "no", "\n";

$mk = $a->m(...);
$r2 = $mk->bindTo($a2, B::class);
echo "method_rebind_scope_is_null: ", $r2 === null ? "yes" : "no", "\n";

$r3 = $mk->bindTo(null);
echo "method_unbind_is_null: ", $r3 === null ? "yes" : "no", "\n";

$r4 = $mk->bindTo(new B);
echo "unrelated_object_is_null: ", $r4 === null ? "yes" : "no", "\n";

$r5 = $mk->bindTo($a2);
echo "same_class_other_instance_is_null: ", $r5 === null ? "yes" : "no", "\n";
echo "r5_call: ", $r5(), "\n";
echo "original_call: ", $mk(), "\n";
echo "function_closure_call: ", $fk(), "\n";
