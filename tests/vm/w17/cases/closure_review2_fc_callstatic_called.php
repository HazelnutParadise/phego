<?php
class A { static function __callStatic($n, $a) { return $n . ":" . static::class; } }
class B extends A { function t() { return Closure::fromCallable("A::zz")(); } static function u() { return Closure::fromCallable("static::yy")(); } }
echo (new B)->t(), "\n";
echo B::u(), "\n";
