<?php
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T{$this->n}\n"; throw new Exception("t{$this->n}"); } }
function f() { foreach ([new T(1)] as $v) { foreach ([new T(2)] as $w) { return 1; } } }
try { f(); } catch (Exception $e) { echo $e->getMessage(), "<", $e->getPrevious() ? $e->getPrevious()->getMessage() : "-", "\n"; }
echo "done\n";
