<?php
$outer = 1000;
$f = fn ($x) => fn ($y) => fn ($z) => $x + $y + $z + $outer;
$g = $f(1);
$h = $g(20);
echo "three_level_sum: ", $h(300), "\n";
echo "reuse_h: ", $h(3000), "\n";
$outer = 5;
echo "outer_changed_after_creation: ", $f(1)(20)(300), "\n";
echo "h_still_uses_1000: ", $h(300), "\n";

$base = 7;
$mk = fn ($a) => fn () => $a * $base;
$c = $mk(3);
echo "arrow_returns_arrow_value: ", $c(), "\n";

$inner = fn ($v) => $v + 1;
$outerClosure = function ($n) use ($inner) {
    $wrap = fn ($m) => $inner($m) * 10;
    return $wrap($n);
};
echo "arrow_inside_closure: ", $outerClosure(4), "\n";

$pick = fn ($flag) => $flag ? (fn () => "yes") : (fn () => "no");
echo "closure_valued_true: ", $pick(true)(), "\n";
echo "closure_valued_false: ", $pick(false)(), "\n";

$counter = 0;
$chain = fn ($a) => fn ($b) => $a . $b . $counter;
echo "chain_strings: ", $chain("p")("q"), "\n";
$counter = 99;
echo "chain_after_outer_change: ", $chain("p")("q"), "\n";
