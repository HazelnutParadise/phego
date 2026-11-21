<?php
class M {
    public function __call($name, $args) {
        echo "__call name=", $name, " args:";
        foreach ($args as $k => $v) {
            echo " [", $k, "]=", $v;
        }
        echo "\n";
        return "call:" . $name;
    }
}

class S {
    public static function __callStatic($name, $args) {
        echo "__callStatic name=", $name, " args:";
        foreach ($args as $k => $v) {
            echo " [", $k, "]=", $v;
        }
        echo "\n";
        return "static:" . $name;
    }
}

$m = new M;
$c1 = $m->undefined(...);
echo "created_c1\n";
echo "result1: ", $c1("a", "b"), "\n";

$c2 = $m->other(...);
echo "result2: ", $c2(named: "n"), "\n";

$name = "dyn";
$c3 = $m->$name(...);
echo "result3: ", $c3(), "\n";

$s = S::undefined(...);
echo "created_s\n";
echo "result4: ", $s(1, 2, 3), "\n";

$t = S::other(...);
echo "result5: ", $t(), "\n";
echo "c1_is_closure: ", $c1 instanceof Closure ? "yes" : "no", "\n";
