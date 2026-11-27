<?php
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T", $this->n, " "; throw new Exception("t".$this->n); } }
function f() { try { try { foreach ([1, new T("b")] as $x) { return "r"; } } finally { echo "F "; } } catch (Exception $q) { echo "Q", $q->getMessage(), " "; } return "fell"; } try { echo f(), "\n"; } catch (Exception $e) { echo "U", $e->getMessage(), "\n"; }
