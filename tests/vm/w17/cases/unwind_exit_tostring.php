<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class S { function __toString(): string { exit("out\n"); } }
function f() { $d = new D(1); try { echo new S; } finally { echo "fin\n"; } }
f();
