--TEST--
Self-assigned arrays are copied and shutdown destructs objects created by destructors
--FILE--
<?php
class P { public $name; function __construct($n){$this->name=$n;} function __destruct(){ global $child; echo "destruct {$this->name}\n"; if ($this->name === 'parent') { $child = new P('child'); } } }
$a = new P('parent'); $b = $a;
$x = [1]; $x[] = $x; echo json_encode($x), "\n";
$y = ['k' => 1]; $y['self'] = $y; echo json_encode($y), "\n";
echo "end\n";
--EXPECT--
[1,[1]]
{"k":1,"self":{"k":1}}
end
destruct parent
destruct child
