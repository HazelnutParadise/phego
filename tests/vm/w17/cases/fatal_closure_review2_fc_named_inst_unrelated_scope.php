<?php
class A { function inst() { return "inst:" . $this::class; } }
class B extends A {}
class X {}
$f = function () { return Closure::fromCallable("A::inst")(); };
$g = Closure::bind($f, new B, X::class);
echo $g(), "\n";
