<?php
class D { function __destruct() { throw new Exception("from dtor"); } }
class C { function m($a) { $d = new D; } }
try {
  (new C)->m(1);
} catch (Exception $e) { echo $e->getTraceAsString(), "\n"; }
