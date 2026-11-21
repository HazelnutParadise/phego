<?php
class D { function __construct(public $n) { if ($n == 2) exit(4); } function __destruct() { echo "~{$this->n}\n"; } }
$a = new D(1);
$b = new D(2);
