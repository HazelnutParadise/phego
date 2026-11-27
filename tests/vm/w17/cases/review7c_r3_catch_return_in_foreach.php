<?php
class D { function __construct(public $n) {} function __destruct() { echo "d{$this->n}\n"; } }
function arr($a, $b) { return [new D($a), new D($b)]; }
function f() { foreach (arr(1,2) as $v) { try { throw new Exception("e"); } catch (Exception $e) { echo "catch\n"; return 1; } } } echo f(), "\n"; echo "end\n";
