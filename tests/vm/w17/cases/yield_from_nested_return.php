<?php
function a() { yield 1; return 'A'; }
function b() { $x = yield from a(); yield 2; return $x . 'B'; }
function c() { $y = yield from b(); echo "c got $y\n"; return $y . 'C'; }
$g = c();
foreach ($g as $v) echo $v, "\n";
echo $g->getReturn(), "\n";
