<?php
class A {
    static function t() { return Closure::fromCallable("self::m"); }
    static function u() { return Closure::fromCallable("static::m"); }
    static function m() { return static::class; }
    function inst() { return isset($this) ? "this" : "nothis"; }
    function viaName() { return Closure::fromCallable("A::inst")(); }
    function viaArray() { return Closure::fromCallable(["A", "inst"])(); }
}
class B extends A { static function p() { return Closure::fromCallable("parent::m"); } }
echo B::t()(), "\n";
echo B::u()(), "\n";
echo B::p()(), "\n";
echo (new A)->viaName(), (new B)->viaArray(), "\n";
