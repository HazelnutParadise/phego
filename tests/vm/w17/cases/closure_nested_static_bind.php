<?php
class A { const F = 1; public $p = 5;
  function a() { return fn() => fn() => $this->p; }
  function b() { return static fn() => isset($this) ? "has" : "none"; }
  function c() { return function () { return static function () { return isset($this) ? "has" : "none"; }; }; }
  function d() { return function () { return function () { return static::class . self::F . $this->p; }; }; } }
class B extends A {}
$o = new B;
echo $o->a()()(), $o->b()(), $o->c()()(), $o->d()()(), "\n";
$f = $o->d()();
$g = Closure::bind($f, new A, A::class);
echo $g(), "\n";
$h = $f->bindTo(null);
