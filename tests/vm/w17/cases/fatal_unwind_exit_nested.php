<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class E { function __destruct() { echo "~E\n"; exit("bye\n"); } }
class T { function __destruct() { echo "~T\n"; throw new Exception("t"); } }
function f() { $d = new D(1); $t = new T; $e = new E; unset($e); echo "not reached\n"; }
f();
