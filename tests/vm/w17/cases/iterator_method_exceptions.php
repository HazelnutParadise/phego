<?php
class It implements Iterator {
    public $fail; private $i = 0;
    function __construct($f) { $this->fail = $f; }
    function current(): mixed { if ($this->fail == 'current') throw new Exception("current"); return $this->i; }
    function key(): mixed { if ($this->fail == 'key') throw new Exception("key"); return $this->i; }
    function next(): void { if ($this->fail == 'next') throw new Exception("next"); $this->i++; }
    function rewind(): void { if ($this->fail == 'rewind') throw new Exception("rewind"); $this->i = 0; }
    function valid(): bool { if ($this->fail == 'valid') throw new Exception("valid"); return $this->i < 2; }
}
foreach (['rewind', 'valid', 'current', 'key', 'next'] as $f) {
    try { foreach (new It($f) as $k => $v) echo "$k=$v "; } catch (Exception $e) { echo "caught ", $e->getMessage(); }
    echo "\n";
}
