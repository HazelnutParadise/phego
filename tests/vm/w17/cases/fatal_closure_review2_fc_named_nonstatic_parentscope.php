<?php
class A { function m() { return "m:" . $this::class; } function t() { return Closure::fromCallable("B::m")(); } }
class B extends A {}
echo "before\n";
echo (new B)->t(), "\n";
