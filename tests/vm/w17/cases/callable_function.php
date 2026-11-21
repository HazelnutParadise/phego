<?php
namespace N;

function add($a, $b = 10) {
    return $a + $b;
}

function bump(&$n) {
    $n++;
    return $n;
}

function counter() {
    static $c = 0;
    $c++;
    return $c;
}

$f = add(...);
echo "ns_call: ", $f(1, 2), "\n";
echo "ns_default: ", $f(1), "\n";
echo "ns_named: ", $f(b: 5, a: 1), "\n";
echo "ns_is_closure: ", $f instanceof \Closure ? "Closure" : "other", "\n";
echo "ns_unpack: ", $f(...[4, 5]), "\n";
echo "ns_unpack_named: ", $f(...["b" => 7, "a" => 1]), "\n";

$inc = bump(...);
$v = 1;
echo "ns_byref: ", $inc($v), " ", $v, "\n";
$v = 10;
echo "ns_byref_again: ", $inc($v), " ", $v, "\n";

$cnt = counter(...);
echo "static_direct: ", counter(), "\n";
echo "static_via_closure: ", $cnt(), "\n";
echo "static_direct_again: ", counter(), "\n";
echo "static_via_closure_again: ", $cnt(), "\n";

echo "fq_direct_call: ", \N\add(2, 3), "\n";
$viaFq = \N\add(...);
echo "fq_fcc_call: ", $viaFq(20), "\n";
