<?php
class D { function __construct(public $n) {} function __destruct() { echo "~{$this->n}\n"; } }
function f(int $x) {}
f(new D(1));
