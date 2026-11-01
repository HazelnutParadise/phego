<?php
namespace Fx;

function outer($p) {
    $a = 1; $b = 2; $c = 3; $d = 4;
    $GLOBALS['closures'][] = function ($x, &$y = null) use ($a, &$b): int { static $n = 0; return $x; };
    $GLOBALS['closures'][] = fn($x) => $x + $a + $b + (fn($y) => $y * $c)(1);
    $GLOBALS['closures'][] = fn() => [$d, function () use ($a) {}, fn() => $c];
    $GLOBALS['closures'][] = static fn(int ...$xs): array => $xs;
    $GLOBALS['closures'][] = function () { return [__FUNCTION__, __METHOD__, __CLASS__]; };
    $GLOBALS['closures'][] = fn() => __FUNCTION__;
}
outer(1);

class Box {
    public function make() {
        $q = 1;
        $GLOBALS['closures'][] = function () use ($q) { return [__FUNCTION__, __METHOD__, __CLASS__, $this]; };
        $GLOBALS['closures'][] = fn() => [__METHOD__, $q, $this];
        $GLOBALS['values'][] = __METHOD__;
        $GLOBALS['values'][] = __FUNCTION__;
        $GLOBALS['values'][] = __CLASS__;
    }
}
(new Box)->make();

$m = 5;
$closures[] = function () use ($m) { return __FUNCTION__; };
$closures[] = fn() => $m;
$closures[] = static function &() use (&$m) { static $k; return $m; };
