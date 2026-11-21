<?php
class D { function __construct(public $n) {} function __destruct() { global $other; echo "~{$this->n} other=", $other === null ? "null" : $other->n, "\n"; } }
$first = new D(1);
$other = new D(2);
echo "end\n";
