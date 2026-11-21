<?php
class A {}
$f = function () { return 1; };
$g = $f->bindTo(null, true);
echo $g === null ? "NULL" : $g(), "\n";
$g = $f->bindTo(null, 1.5);
echo $g === null ? "NULL" : $g(), "\n";
$g = Closure::bind($f, null, new A);
echo $g(), "\n";
$g = Closure::bind($f, null, 5);
echo $g === null ? "NULL" : $g(), "\n";
$g = Closure::bind($f, null, false);
echo $g === null ? "NULL" : $g(), "\n";
