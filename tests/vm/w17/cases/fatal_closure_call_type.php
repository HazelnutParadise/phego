<?php
class A {}
$f = function (int $a) { return 1; };
echo "x\n";
$f->call(new A, []);
