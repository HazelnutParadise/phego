<?php
abstract class A { abstract function x(); }
class B extends A { function x() { return "bx"; } }
$f = [new B, "x"];
echo $f(), "\n";
$g = ["A", "x"];
$g();
