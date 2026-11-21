<?php
class A { function __call($n, $a) { return "call:" . $n; } static function __callStatic($n, $a) { return "cs:" . $n; } private function priv() { return "priv"; } function get() { return $this->priv(...); } }
$o = new A;
$f = $o->zzz(...); $g = A::yyy(...); $h = $o->priv(...);
echo $f(), $g(), $h(), $o->get()(), "\n";
echo $f == $o->zzz(...) ? "eq" : "ne", $f == $o->www(...) ? "eq" : "ne", "\n";
$k = Closure::fromCallable([$o, "priv"]);
echo $k(), "\n";
$z = Closure::fromCallable("A::nope2");
echo $z(), "\n";
Closure::fromCallable([$o, 5]);
