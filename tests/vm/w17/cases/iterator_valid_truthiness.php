<?php
class It implements Iterator {
    private $i = 0;
    function current(): mixed { return $this->i; } function key(): mixed { return "k" . $this->i; }
    function next(): void { $this->i++; } function rewind(): void { $this->i = 0; }
    #[\ReturnTypeWillChange] function valid() { return [0 => "1", 1 => "a", 2 => 0, 3 => ""][$this->i] ?? null; }
}
foreach (new It as $k => $v) echo "$k=$v\n";
