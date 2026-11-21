<?php
class A { function f() { return empty($this) ? "e" : "ne"; } static function g() { return empty($this) ? "e" : "ne"; } }
echo (new A)->f(), A::g(), "\n";
