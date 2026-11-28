<?php
class A implements IteratorAggregate { function getIterator(): Iterator { echo "getIterator\n"; return (function () { yield 'k' => 'v'; return 'ignored'; })(); } }
function g() { $r = yield from new A; echo "r=", $r === null ? "null" : $r, "\n"; }
foreach (g() as $k => $v) echo "$k => $v\n";
