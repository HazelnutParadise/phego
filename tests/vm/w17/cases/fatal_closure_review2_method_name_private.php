<?php
class A { public $x = "X"; private function pp() { return "private:" . (isset($this) ? "this" : "nothis"); } }
echo "before\n";
$f = "pp";
echo $f(), "\n";
