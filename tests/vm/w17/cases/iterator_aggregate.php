<?php
class Agg implements IteratorAggregate {
    function getIterator(): Iterator { echo "getIterator\n"; return (function () { yield 'x' => 1; yield 'y' => 2; })(); }
}
foreach (new Agg as $k => $v) echo "$k=$v\n";
class Agg2 implements IteratorAggregate { function getIterator(): Traversable { return new Agg; } }
foreach (new Agg2 as $k => $v) echo "$k=$v\n";
class Bad implements IteratorAggregate { function getIterator(): Traversable { return null; } }
try { foreach (new Bad as $v) {} } catch (TypeError $e) { echo $e->getMessage(), "\n"; }
