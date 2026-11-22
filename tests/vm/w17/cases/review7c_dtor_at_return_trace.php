<?php
class D { function __destruct() { throw new Exception("from dtor"); } }
function f() { $d = new D; return 5; }
try {
  $x = f();
} catch (Exception $e) { echo $e->getLine(), "\n", $e->getTraceAsString(), "\n"; }
