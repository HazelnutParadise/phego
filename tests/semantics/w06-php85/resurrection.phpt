--TEST--
A destructor can resurrect its object without running twice
--FILE--
<?php
class R { public static $keep; public $n = 0; function __destruct(){ $this->n++; echo "destruct {$this->n}\n"; self::$keep = $this; } }
$r = new R(); unset($r);
echo "kept ", R::$keep->n, "\n";
R::$keep = null;
echo "cleared\n";
--EXPECT--
destruct 1
kept 1
cleared
