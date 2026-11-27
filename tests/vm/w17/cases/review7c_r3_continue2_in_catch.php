<?php
class D { function __construct(public $n) {} function __destruct() { echo "d{$this->n}\n"; } }
function arr($a, $b) { return [new D($a), new D($b)]; }
function f() { foreach (arr(1,2) as $v) { foreach (arr(3,4) as $w) { try { throw new Exception("e"); } catch (Exception $e) { echo "c{$v->n}{$w->n}\n"; continue 2; } } } echo "after\n"; } f(); echo "end\n";
