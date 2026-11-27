<?php
class D { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~", $this->n, " "; } }
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T", $this->n, " "; throw new Exception("t".$this->n); } }
try { foreach ([new D("a"), new T("b")] as $v) { throw new Exception("x"); } } catch (Exception $q) { echo "Q", $q->getMessage(), " P", $q->getPrevious()->getMessage(), " "; } echo "end\n";
