<?php
class It implements Iterator {
    private $i = 0;
    function current(): mixed { return $this->i; } function key(): mixed { return $this->i; }
    function next(): void { $this->i++; } function rewind(): void { $this->i = 0; } function valid(): bool { return $this->i < 2; }
}
foreach (new It as &$v) echo "$v\n";
