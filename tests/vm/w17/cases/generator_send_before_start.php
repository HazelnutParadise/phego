<?php
function gen() { echo "start\n"; $a = yield 'first'; echo "a=$a\n"; $b = yield 'second'; echo "b=$b\n"; return "$a$b"; }
$g = gen();
echo $g->send("X"), "\n";
echo $g->send("Y"), "\n";
echo $g->getReturn(), "\n";
