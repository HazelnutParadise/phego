<?php
function gen() { yield 1; }
class A { function m() { yield 2; } }
$d = gen()->__debugInfo(); echo $d['function'], "\n";
$d = (new A)->m()->__debugInfo(); echo $d['function'], "\n";
