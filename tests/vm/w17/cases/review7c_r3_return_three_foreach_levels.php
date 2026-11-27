<?php
class D { function __construct(public $n) {} function __destruct() { echo "d{$this->n}\n"; } }
function arr($a, $b) { return [new D($a), new D($b)]; }
function f() { foreach (arr(1,2) as $a) { foreach (arr(3,4) as $b) { foreach (arr(5,6) as $c) { return 0; } } } } f(); echo "end\n";
