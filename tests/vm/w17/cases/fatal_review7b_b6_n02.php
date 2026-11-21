<?php
class A { function __destruct() { echo "dtor\n"; } }
function f() { global $x; $x = 1; nope(); }
$a = new A(f());
