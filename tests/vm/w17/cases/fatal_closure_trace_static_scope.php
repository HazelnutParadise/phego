<?php
class A { static function smk() { return static function ($a) { return nope($a); }; } }
echo "x\n";
$f = A::smk();
$f([1,2]);
