<?php
class T { public $n; function __construct($n){$this->n=$n;} function __destruct(){ echo "~T", $this->n, " "; throw new Exception("t".$this->n); } }
function f() { try { foreach ([1, new T("b")] as $x) { return "r"; } } catch (Exception $q) { echo "Q", $q->getMessage(), " "; } finally { echo "F "; } return "fell"; } try { echo f(), "\n"; } catch (Exception $e) { echo "U", $e->getMessage(), "\n"; }
