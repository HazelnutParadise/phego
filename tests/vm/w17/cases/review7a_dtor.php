<?php
class A { public $o; function __destruct() { echo "~\n"; } }
$n = 1;
$a = new A; $b = new A; $a->o = $b;
echo "end\n";
