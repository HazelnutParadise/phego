<?php
class It implements Iterator {
    private $i = 0; private $d = ['a', 'b', 'c'];
    function current(): mixed { echo "current\n"; return $this->d[$this->i]; }
    function key(): mixed { echo "key\n"; return $this->i; }
    function next(): void { echo "next\n"; $this->i++; }
    function rewind(): void { echo "rewind\n"; $this->i = 0; }
    function valid(): bool { echo "valid\n"; return $this->i < 3; }
}
foreach (new It as $k => $v) echo "$k=$v\n";
echo "--\n";
foreach (new It as $v) echo "$v\n";
