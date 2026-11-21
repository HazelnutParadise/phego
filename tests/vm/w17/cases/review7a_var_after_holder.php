<?php
class D { public $c; function __construct(public $n) {} function __destruct() { echo "~{$this->n}\n"; } }
$holder = new D(1);
$child = new D(2);
$holder->c = $child;
