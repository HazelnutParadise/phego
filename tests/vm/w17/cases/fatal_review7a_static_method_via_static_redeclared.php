<?php
class A { private static function m() { return "A::m"; } static function c() { return static::m(); } }
class B extends A { private static function m() { return "B::m"; } }
echo B::c(), "\n";
