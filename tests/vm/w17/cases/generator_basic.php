<?php
function gen() { echo "start\n"; yield 1; echo "mid\n"; yield 2; echo "end\n"; }
$g = gen();
echo "created\n";
foreach ($g as $k => $v) { echo "$k => $v\n"; }
echo $g instanceof Generator ? "gen" : "no", "\n"; echo $g instanceof Traversable ? "trav" : "no", "\n"; echo $g instanceof Iterator ? "iter" : "no", "\n";
