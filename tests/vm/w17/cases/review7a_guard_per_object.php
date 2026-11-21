<?php
class A { public $other; function __get($n) { echo "get $n on ", $this === $GLOBALS_less ?? "", "\n"; return $this->other ? $this->other->$n : "end"; } }
$GLOBALS_less = null;
$a = new A; $b = new A; $a->other = $b;
echo $a->x, "\n";
