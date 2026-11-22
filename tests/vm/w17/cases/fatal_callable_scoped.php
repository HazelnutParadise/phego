<?php
class P { protected function hid() { return "P"; } public static function make() { return function () { return static::class . ":" . get(); }; } }
function get() { return "g"; }
class C extends P { function viaParent() { return parent::hid(...); } function viaSelf() { return self::hid(...); } }
$c = new C;
echo $c->viaParent()(), $c->viaSelf()(), "\n";
echo C::make()(), P::make()(), "\n";
$m = C::make();
echo $m->bindTo(null, P::class)(), "\n";
$x = "hid";
echo [$c, "viaSelf"]()(), "\n";
[$c, "hid"]();
