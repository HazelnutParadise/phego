<?php
class D { function __construct(public $n) { echo "ctor\n"; $q = [] + 1; } function __destruct() { echo "~{$this->n}\n"; } }
function f() { return new D(1); }
f();
