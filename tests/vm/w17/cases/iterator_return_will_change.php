<?php
class It implements Iterator {
    private $i = 0;
    #[\ReturnTypeWillChange]
    function current() { return $this->i; }
    #[ReturnTypeWillChange]
    function key() { return $this->i; }
    function next(): void { $this->i++; }
    function rewind(): void { $this->i = 0; }
    function valid(): bool { return $this->i < 2; }
}
foreach (new It as $k => $v) echo "$k $v\n";
