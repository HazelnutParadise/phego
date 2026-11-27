<?php
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T{$this->n}\n"; throw new Exception("t{$this->n}"); } }
function f() { $a = new T(1); $b = new T(2); return 1; }
try { f(); } catch (Exception $e) { echo $e->getMessage(), "<", $e->getPrevious() ? $e->getPrevious()->getMessage() : "-", "\n"; }
echo "done\n";
