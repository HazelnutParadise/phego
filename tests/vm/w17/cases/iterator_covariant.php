<?php
class It implements Iterator {
    private $i = 0;
    function current(): int { return $this->i; }
    function key(): int|string { return $this->i; }
    function next(): void { $this->i++; }
    function rewind(): void { $this->i = 0; }
    function valid(): bool { return $this->i < 2; }
}
foreach (new It as $k => $v) echo "$k $v\n";
abstract class Base implements IteratorAggregate {}
class Impl extends Base { function getIterator(): Generator { yield 1; } }
foreach (new Impl as $v) echo $v, "\n";
interface MyIt extends Iterator {}
echo (new It) instanceof Traversable ? "t" : "f", "\n";
