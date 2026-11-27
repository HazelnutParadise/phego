<?php
class D { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~D{$this->n}\n"; } }
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T{$this->n}\n"; throw new Exception("t{$this->n}"); } }
function ch($e){ $s=""; while($e){ $s.=$e->getMessage()."<"; $e=$e->getPrevious(); } return $s; }
function f() { foreach ([new D(2)] as $v) { $v=null; try { foreach ([new D(1)] as $w) { $w=null; foreach ([new D(3)] as $u) { $u=null; return "r"; } } } finally { echo "F\n"; } } }
try { $r = f(); echo "ret\n"; $r = null; } catch (Exception $e) { echo "G ", ch($e), "\n"; }
echo "done\n";
