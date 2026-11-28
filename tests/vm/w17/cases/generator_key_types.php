<?php
class K { function __toString(): string { return "K"; } }
function gen() { $o = new K; yield $o => 1; yield [1] => 2; yield null => 3; yield 2.5 => 4; yield 'x'; yield 'y'; }
foreach (gen() as $k => $v) { echo $v, ": ", is_k($k), "\n"; }
function is_k($k) { if ($k instanceof K) return "object"; if ($k === [1]) return "array"; if ($k === null) return "null"; if ($k === 2.5) return "float"; return "int $k"; }
