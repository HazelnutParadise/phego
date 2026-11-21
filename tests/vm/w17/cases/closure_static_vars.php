<?php
// closure_static_vars: static locals in closures are per closure object
$counter = function () {
    static $n = 0;
    $n = $n + 1;
    return $n;
};
echo "first_call: ", $counter(), "\n";
echo "second_call: ", $counter(), "\n";

$other = function () {
    static $n = 0;
    $n = $n + 1;
    return $n;
};
echo "other_own_static: ", $other(), "\n";
echo "counter_still: ", $counter(), "\n";

$made = [];
for ($i = 0; $i < 3; $i = $i + 1) {
    $made[$i] = function () {
        static $hits = 0;
        $hits = $hits + 1;
        return $hits;
    };
}
echo "loop_a_1: ", $made[0](), "\n";
echo "loop_a_2: ", $made[0](), "\n";
echo "loop_b_1: ", $made[1](), "\n";
echo "loop_c_1: ", $made[2](), "\n";
echo "loop_a_3: ", $made[0](), "\n";

$base = function () {
    static $k = 10;
    $k = $k + 1;
    return $k;
};
echo "base_1: ", $base(), "\n";
echo "base_2: ", $base(), "\n";
$copy = clone $base;
echo "copy_1: ", $copy(), "\n";
echo "base_3: ", $base(), "\n";
echo "copy_2: ", $copy(), "\n";
echo "copy_3: ", $copy(), "\n";
echo "base_4: ", $base(), "\n";

$calc = function () {
    static $w = 2 ** 3;
    $w = $w + 1;
    return $w;
};
echo "expr_init_1: ", $calc(), "\n";
echo "expr_init_2: ", $calc(), "\n";

function factory() {
    return function () {
        static $z = 0;
        $z = $z + 1;
        return $z;
    };
}
$fa = factory();
$fb = factory();
echo "factory_a: ", $fa(), $fa(), "\n";
echo "factory_b: ", $fb(), "\n";
