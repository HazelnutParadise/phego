<?php
function gen() { echo "start\n"; $x = yield 1; echo "x=$x\n"; $y = yield 2; echo "y=$y\n"; }
$g = gen();
echo $g->send("A"), "\n";
echo "--\n";
$g2 = gen();
$g2->next();
echo $g2->current(), "\n";
$g2->next();
echo $g2->current() === null ? "null" : "x", "\n";
