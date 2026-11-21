<?php
class A { public $o; }
$n = 1;
$a = new A; $b = new A; $c = new A; $a->o = $b; $b->o = $c;
echo "end\n";
