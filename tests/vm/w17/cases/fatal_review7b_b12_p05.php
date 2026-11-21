<?php
class P { static function __callStatic($n, $a) { echo "cs $n\n"; } }
class C extends P { static function f() { parent::__construct(); } }
C::f();
