<?php
class D { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~", $this->n, "\n"; } }
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T", $this->n, "\n"; throw new Exception("T".$this->n); } }
class P { public $n; public $p; function __construct($n,$p){$this->n=$n;$this->p=$p;} function __destruct(){ echo "~P", $this->n, "\n"; throw new Exception("P".$this->n, 0, $this->p); } }
function pe($e){ echo get_class_safe($e), $e->getMessage(); $p=$e->getPrevious(); while($p!==null){ echo " <- ", $p->getMessage(); $p=$p->getPrevious(); } echo "\n"; }
function get_class_safe($e){ return ""; }
function f(){ $a=new T(1); $b=new T(2); exit("bye\n"); } f();
