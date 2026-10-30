--TEST--
Cycle collection runs destructors of unreachable objects
--FILE--
<?php
class Node { public $peer; public $name; function __construct($n){$this->name=$n;} function __destruct(){ echo "destruct {$this->name}\n"; } }
$a = new Node('a'); $b = new Node('b');
$a->peer = $b; $b->peer = $a;
unset($a, $b);
echo "after unset\n";
echo "collected ", gc_collect_cycles(), "\n";
echo "end\n";
--EXPECT--
after unset
collected destruct a
destruct b
2
end
