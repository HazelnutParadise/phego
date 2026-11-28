<?php
interface I extends Traversable {}
class C implements I, IteratorAggregate { function getIterator(): Iterator { return (function () { yield 7; })(); } }
foreach (new C as $v) echo $v, "\n";
abstract class X implements Iterator {}
echo "ok\n";
