<?php
class Base { protected $p = "base"; }
class L extends Base { function f($o) { return $o->p; } }
class R extends Base { protected $p = "r"; }
echo (new L)->f(new R), "\n";
