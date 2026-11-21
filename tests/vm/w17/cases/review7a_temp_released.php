<?php
class D { function __construct(public $n) {} function __destruct() { echo "~{$this->n}\n"; } }
$c = clone (new D(1));
echo "after\n";
