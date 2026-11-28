<?php
$gen = (function () { $x = 1; while (true) { $y = yield $x; if ($y === null) break; $x += $y; } return $x; })();
foreach ([1, 2, 3] as $n) { $gen->send($n); }
echo $gen->current(), "\n";
$gen->next();
echo $gen->getReturn(), "\n";
function counter() { $i = 0; while (true) { $cmd = yield $i++; if ($cmd === 'stop') return "stopped at $i"; } }
$c = counter();
foreach ($c as $v) { if ($v == 3) $c->send('stop'); echo $v, "\n"; }
echo $c->getReturn(), "\n";
