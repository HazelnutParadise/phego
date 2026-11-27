<?php
class D { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~D{$this->n}\n"; } }
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T{$this->n}\n"; throw new Exception("t{$this->n}"); } }
function ch($e){ $s=""; while($e){ $s.=$e->getMessage()."@".$e->getLine()."<"; $e=$e->getPrevious(); } return $s; }
class K { function f() {  foreach ([new D(1), new D(2)] as $v) { echo "o\n"; foreach ([new D(3)] as $w) { try { echo "in\n"; return $v; } finally { echo "F3\n"; } } } return "end"; } }
function g() { $k=new K; try { $r=$k->f(); echo "r ", ($r instanceof D || $r instanceof T || $r instanceof K) ? "obj" : $r, "\n"; $r=null; } catch (Exception $e) { echo "G ".ch($e)."\n"; } echo "g end\n"; }
g(); echo "done\n";
