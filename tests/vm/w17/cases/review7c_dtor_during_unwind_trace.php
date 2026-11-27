<?php
class D { function __destruct() { echo (new Exception("t"))->getTraceAsString(), "\n"; } }
function f() { $d = new D; throw new Exception("orig"); }
try {
  f();
} catch (Exception $e) { echo "caught\n"; }
