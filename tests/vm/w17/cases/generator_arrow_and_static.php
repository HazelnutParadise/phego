<?php
$a = 3;
$f = fn($n) => yield $n * $a;
$g = $f(2); echo $g->current(), "\n"; $g->next(); echo $g->getReturn() === null ? "null" : "x", "\n";
$h = fn() => yield from [1, 2];
foreach ($h() as $k => $v) echo "$k:$v\n";
$s = static function () { yield 1; };
foreach ($s() as $v) echo $v, "\n";
