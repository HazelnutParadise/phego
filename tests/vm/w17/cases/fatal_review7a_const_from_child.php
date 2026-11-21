<?php
class A { private const X = 1; }
class B extends A { function f() { return self::X; } }
echo (new B)->f();
