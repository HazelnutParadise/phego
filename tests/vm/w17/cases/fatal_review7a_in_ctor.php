<?php
class D { function __construct(public $n) { echo "ctor\n"; $x = null; $x->foo(); } function __destruct() { echo "~{$this->n}\n"; } }
$a = new D(1);
