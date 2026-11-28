<?php
$e = new Exception("E");
class T1 { function __destruct() { echo "~T1\n"; throw new Exception("T1"); } }
class T2 { function __destruct() { global $e; echo "~T2\n"; throw $e; } }
function f() { global $e; $a = new T1; $b = new T2; throw $e; }
try { f(); } catch (Exception $x) {
  echo $x->getMessage(), "\n";
  $p = $x->getPrevious();
  echo $p ? $p->getMessage() : "none", "\n";
}
