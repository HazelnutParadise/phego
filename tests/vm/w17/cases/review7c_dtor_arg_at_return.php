<?php
class D { function __destruct() { echo (new Exception("t"))->getTraceAsString(), "\n"; } }
function f($d) { return 1; }
function g() { f(new D); echo "g after\n"; }
g();
