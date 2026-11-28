<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen($n) { $d = new D($n); try { yield 1; } finally { echo "finally $n\n"; } }
$a = gen(1); $a->current();
$b = gen(2); $b->current();
$c = new D(3);
echo "end\n";
