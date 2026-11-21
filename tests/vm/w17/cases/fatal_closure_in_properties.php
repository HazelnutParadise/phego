<?php
class A { public $fn; public static $sfn; function __construct() { $this->fn = fn($x) => $x * 2; self::$sfn = fn($x) => $x + 1; } }
$o = new A;
echo ($o->fn)(2), (A::$sfn)(2), $o->fn->__invoke(3), "\n";
$arr = ["k" => fn() => "ak"];
echo $arr["k"](), "\n";
$fs = [fn() => 1, fn() => 2];
foreach ($fs as $f) { echo $f(); }
echo "\n";
echo (function () { return fn() => "deep"; })()(), "\n";
$o->fn();
