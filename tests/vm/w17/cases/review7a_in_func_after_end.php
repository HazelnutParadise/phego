<?php
class D { function __destruct() { f(); } }
function f() { global $q; echo "q=", $q === null ? "null" : $q, "\n"; }
$q = 1;
$d = new D;
echo "end\n";
