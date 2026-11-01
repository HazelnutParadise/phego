--TEST--
Objects resurrected during cycle collection survive and are not destructed twice
--FILE--
<?php
class C { public $self; public $name; function __construct($n){$this->name=$n;} function __destruct(){ global $saved; echo "destruct {$this->name}\n"; if ($this->name === 'keep') { $saved = $this; } } }
$k = new C('keep'); $k->self = $k; unset($k);
$d = new C('drop'); $d->self = $d; unset($d);
echo "collected ", gc_collect_cycles(), "\n";
echo "saved ", $saved->name, " self ok ", var_export($saved->self === $saved, true), "\n";
$saved = null;
echo "collected again ", gc_collect_cycles(), "\n";
$arr = []; $arr[0] = &$arr; unset($arr); echo "array cycle ", gc_collect_cycles(), "\n";
echo "end\n";
--EXPECT--
collected destruct keep
destruct drop
1
saved keep self ok true
collected again 1
array cycle 1
end
