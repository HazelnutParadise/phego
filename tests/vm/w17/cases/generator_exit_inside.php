<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen() { $d = new D(1); try { yield 1; exit("bye\n"); } finally { echo "no finally on exit\n"; } }
$x = new D(2);
foreach (gen() as $v) { echo $v, "\n"; }
