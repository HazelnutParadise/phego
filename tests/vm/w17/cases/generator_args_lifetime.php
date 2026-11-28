<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen($a, $b) { echo "start\n"; yield 1; echo "a=", $a->n, "\n"; }
$g = gen(new D(1), new D(2));
echo "created\n";
$g->current();
$g->next();
echo "finished\n";
unset($g);
echo "end\n";
