<?php
class A { function f() { return isset($this) ? $this::class : "none"; } }
class B extends A { function g() { return A::f(); } }
echo (new B)->g(), "\n";
