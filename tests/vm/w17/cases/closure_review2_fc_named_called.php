<?php
class A { static function m() { return static::class; } function inst() { return get_class_name($this); } }
function get_class_name($o) { return $o::class; }
class B extends A { function t() { return Closure::fromCallable("A::m")(); } function u() { return Closure::fromCallable(["A", "m"])(); } }
echo (new B)->t(), "\n";
echo (new B)->u(), "\n";
