<?php
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T", $this->n, "\n"; throw new Exception("T".$this->n); } }
function f(){ $a=new T(1); exit("bye\n"); } f();
