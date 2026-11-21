<?php
class A {
    public $v = "av";

    public function make() {
        return static function () {
            return "static inside " . self::class;
        };
    }
}

$sc = static function () {
    return "static closure ran";
};

$obj = new A;
$bad = $sc->bindTo($obj);
echo "bind_result_is_null: ", $bad === null ? "yes" : "no", "\n";

$ok = $sc->bindTo(null, A::class);
echo "null_scope_bind: ", $ok(), "\n";

$ok2 = Closure::bind($sc, null, A::class);
echo "static_bind_with_scope: ", $ok2(), "\n";

$mk = $obj->make();
echo "method_static_closure: ", $mk(), "\n";

$null = $mk->bindTo($obj);
echo "method_static_rebind_is_null: ", $null === null ? "yes" : "no", "\n";
echo "bad_is_set: ", isset($bad) ? "set" : "unset", "\n";
echo "end\n";
