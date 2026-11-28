<?php
function g($n) { for ($i = 0; $i < $n; $i++) { yield $i => $i * $i; } }
foreach (g(3) as $a) { foreach (g(2) as $k => $b) { echo "$a $k $b\n"; } }
function keys() { yield [1, 2] => 'arr'; yield 1.5 => 'float'; yield true => 'bool'; }
foreach (keys() as $k => $v) { echo gettype_s($k), " $v\n"; }
function gettype_s($k) { if ($k === true) return "true"; if ($k === 1.5) return "1.5"; if ($k === [1, 2]) return "array"; return "?"; }
