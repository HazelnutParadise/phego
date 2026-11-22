<?php
// closure_returns_closure: currying, counter factories, composition
$curry = function ($a) {
    return function ($b) use ($a) {
        return function ($c) use ($a, $b) {
            return $a . $b . $c;
        };
    };
};
echo "curry_full: ", $curry("x")("y")("z"), "\n";
$step1 = $curry("1");
$step2 = $step1("2");
echo "curry_reuse_a: ", $step2("3"), "\n";
echo "curry_reuse_b: ", $step2("4"), "\n";

function makeCounter() {
    $count = 0;
    return function () use (&$count) {
        $count = $count + 1;
        return $count;
    };
}
$c1 = makeCounter();
$c2 = makeCounter();
echo "counter1_a: ", $c1(), "\n";
echo "counter1_b: ", $c1(), "\n";
echo "counter2_a: ", $c2(), "\n";
echo "counter1_c: ", $c1(), "\n";
echo "immediate_return: ", makeCounter()(), "\n";

function compose($f, $g) {
    return function ($x) use ($f, $g) {
        return $f($g($x));
    };
}
$inc = function ($n) { return $n + 1; };
$dbl = function ($n) { return $n * 2; };
echo "compose_inc_dbl: ", compose($inc, $dbl)(5), "\n";
echo "compose_dbl_inc: ", compose($dbl, $inc)(5), "\n";
$pipeline = compose(compose($inc, $inc), $dbl);
echo "compose_nested: ", $pipeline(1), "\n";

$makers = [];
foreach (["a", "b"] as $tag) {
    $makers[$tag] = function () use ($tag) {
        return function ($suffix) use ($tag) {
            return $tag . $suffix;
        };
    };
}
echo "maker_a: ", $makers["a"]()("!"), "\n";
echo "maker_b: ", $makers["b"]()("?"), "\n";
