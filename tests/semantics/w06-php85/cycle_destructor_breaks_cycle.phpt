--TEST--
Cycle members freed by destructors are not counted as collected
--FILE--
<?php
class N { public $peer; function __destruct(){ $this->peer = null; } }
$a = new N; $b = new N; $a->peer = $b; $b->peer = $a; unset($a, $b);
echo "collected ", gc_collect_cycles(), "\n";
--EXPECT--
collected 1
