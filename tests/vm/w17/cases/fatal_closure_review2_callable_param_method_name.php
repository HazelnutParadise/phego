<?php
class A { static function zq() {} }
function f(callable $c) { return "ok"; }
echo "before\n";
echo f("zq"), "\n";
