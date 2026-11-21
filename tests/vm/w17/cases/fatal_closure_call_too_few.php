<?php
class A {}
$f = function ($a, $b) { return 1; };
echo "x\n";
$f->call(new A, 1);
