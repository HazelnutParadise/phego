<?php
function gen() { yield 1; }
$g = gen();
foreach (['Traversable', 'Iterator', 'IteratorAggregate', 'Generator'] as $c) echo $c, ": ", $g instanceof $c ? "yes" : "no", "\n";
class A implements IteratorAggregate { function getIterator(): Iterator { return gen(); } }
echo (new A) instanceof Traversable ? "yes" : "no", "\n";
function take(Traversable $t) { foreach ($t as $v) echo "took $v\n"; }
take(gen()); take(new A);
function it(iterable $i) { foreach ($i as $v) echo "it $v\n"; }
it([9]); it(gen());
try { take([1]); } catch (TypeError $e) { echo $e->getMessage(), "\n"; }
