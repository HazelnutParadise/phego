<?php
// use_ref_outlives_scope: a by-reference capture keeps a function's local alive
function makeCounter() {
    $count = 0;
    return function () use (&$count) {
        $count = $count + 1;
        return $count;
    };
}
$c = makeCounter();
echo "counter_1: ", $c(), "\n";
echo "counter_2: ", $c(), "\n";
echo "counter_3: ", $c(), "\n";

function pair() {
    $shared = "start";
    $get = function () use (&$shared) {
        return $shared;
    };
    $set = function ($v) use (&$shared) {
        $shared = $v;
    };
    return [$get, $set];
}
$p = pair();
$getter = $p[0];
$setter = $p[1];
echo "pair_initial: ", $getter(), "\n";
$setter("changed");
echo "pair_after_set: ", $getter(), "\n";

$q = pair();
echo "pair2_initial: ", $q[0](), "\n";
$q[1]("other");
echo "pair2_after_set: ", $q[0](), "\n";
echo "pair1_unchanged: ", $getter(), "\n";

$c2 = makeCounter();
echo "counter2_1: ", $c2(), "\n";
echo "counter_still_4: ", $c(), "\n";

function gen() {
    $n = 100;
    $bump = function () use (&$n) {
        $n = $n + 1;
    };
    $bump();
    $bump();
    return $bump;
}
$g = gen();
$g();
echo "gen_bump_after_return: ", $g() === null ? "null" : "other", "\n";
