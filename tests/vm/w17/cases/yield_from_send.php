<?php
function inner() { $x = yield 1; echo "inner got $x\n"; $y = yield 2; echo "inner got $y\n"; return 9; }
function outer() { $r = yield from inner(); echo "outer r=$r\n"; $z = yield 3; echo "outer got $z\n"; }
$g = outer();
echo $g->current(), "\n";
echo $g->send("A"), "\n";
echo $g->send("B"), "\n";
$g->send("C");
