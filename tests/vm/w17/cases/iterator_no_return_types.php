<?php
class It implements Iterator {
    private $i = 0;
    function current() { return $this->i; }
    function key() { return $this->i; }
    function next() { $this->i++; }
    function rewind() { $this->i = 0; }
    function valid() { return $this->i < 2; }
}
foreach (new It as $v) echo "$v\n";
