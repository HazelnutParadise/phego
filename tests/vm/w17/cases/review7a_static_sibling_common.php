<?php
class A { protected static $s = "A"; }
class L extends A { static function r() { return R::$s; } }
class R extends A { protected static $s = "R"; }
echo L::r(), "\n";
