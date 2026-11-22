<?php
// try_catch_closure_scope: try/catch inside closures and arrow functions; captured variables by value and by reference changed inside catch
class Miss2 extends Exception {}

$count = 0;
$log = "start";
$byValue = 10;

$risky = function ($n) use (&$count, $byValue, &$log) {
    try {
        $count++;
        if ($n > 1) {
            throw new Miss2("n=" . $n);
        }
        $byValue = 99;
        return "ok " . $n;
    } catch (Miss2 $e) {
        $log = "caught " . $e->getMessage();
        $byValue = 77;
        return "handled";
    }
};

$r = $risky(1);
echo "1: ", $r, "\n";
$r = $risky(2);
echo "2: ", $r, "\n";
echo "count: ", $count, "\n";
echo "log: ", $log, "\n";
echo "byValue outside: ", $byValue, "\n";

$wrap = fn($v) => $risky($v);
$r = $wrap(1);
echo "3 via arrow: ", $r, "\n";
$r = $wrap(2);
echo "4 via arrow: ", $r, "\n";

$thrower = function ($m) {
    throw new Miss2($m);
};
$caller = fn() => $thrower("from arrow");
try {
    $r = $caller();
    echo "not reached: ", $r, "\n";
} catch (Miss2 $e) {
    echo "5 caught through arrow: ", $e->getMessage(), "\n";
}
echo "end count: ", $count, "\n";
