<?php
class A { public ?int $i = 1; function __get($n) { echo "get $n\n"; $x = null; return $x; } }
$o = new A; unset($o->i); $o->i[0] = 1; echo "end\n";
