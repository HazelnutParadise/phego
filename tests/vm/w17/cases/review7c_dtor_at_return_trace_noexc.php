<?php
class D { function __destruct() { echo (new Exception("t"))->getTraceAsString(), "\n"; } }
function f($a) { $d = new D; return 1; }
function g() { return f(7); }
g();
