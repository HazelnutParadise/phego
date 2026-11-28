<?php
class It implements Iterator {
    private $i = 0;
    function current(): mixed { echo "current\n"; return $this->i * 10; } function key(): mixed { echo "key\n"; return "k{$this->i}"; }
    function next(): void { echo "next\n"; $this->i++; } function rewind(): void { echo "rewind\n"; $this->i = 0; } function valid(): bool { echo "valid\n"; return $this->i < 2; }
}
function g() { $r = yield from new It; echo "r=", $r === null ? "null" : $r, "\n"; }
foreach (g() as $k => $v) echo "$k => $v\n";
