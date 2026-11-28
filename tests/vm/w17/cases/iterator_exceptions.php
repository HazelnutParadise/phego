<?php
class It implements Iterator {
    private $i = 0;
    function current(): mixed { if ($this->i == 1) throw new Exception("cur"); return $this->i; }
    function key(): mixed { return $this->i; } function next(): void { $this->i++; }
    function rewind(): void { $this->i = 0; } function valid(): bool { return $this->i < 3; }
}
try { foreach (new It as $v) echo "$v\n"; } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; }
