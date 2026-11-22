<?php
class D { function __destruct() { echo (new Exception)->getTraceAsString(), "\n"; } }
$f = function ($a) { $d = new D; return $a; };
echo $f->__invoke(5), "\n";
