<?php
class A { public $other; }
$a = new A; $b = new A; $a->other = $b;
echo "end\n";
