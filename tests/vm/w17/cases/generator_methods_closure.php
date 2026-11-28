<?php
class A { private $v = 10; function gen($n) { for ($i = 0; $i < $n; $i++) yield $this->v + $i; } static function sgen() { yield static::class; } }
class B extends A {}
foreach ((new A)->gen(3) as $v) echo $v, "\n";
foreach (B::sgen() as $v) echo $v, "\n";
$f = function ($a) { yield $a; yield $a * 2; };
foreach ($f(3) as $v) echo $v, "\n";
$x = 7;
$h = fn() => yield $x;
foreach ($h() as $k => $v) echo "$k:$v\n";
