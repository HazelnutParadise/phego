<?php
class A { public $other; }
$x = null;
$a = new A; $b = new A; $a->other = $b;
echo "end\n";
