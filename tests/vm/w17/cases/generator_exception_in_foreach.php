<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen() { $d = new D(1); yield 1; throw new Exception("x"); }
try { foreach (gen() as $v) { echo $v, "\n"; } } catch (Exception $e) { echo "caught\n"; }
echo "after\n";
