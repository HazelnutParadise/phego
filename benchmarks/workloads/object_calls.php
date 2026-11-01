<?php
// Method calls, property writes and object allocation.
class Counter {
    var $value = 0;
    function add($n) { $this->value = ($this->value + $n) % 1000003; return $this; }
}
$total = 0;
for ($j = 0; $j < 200; $j++) {
    $c = new Counter();
    for ($i = 0; $i < 5000; $i++) { $c->add($i + $j); }
    $total = ($total + $c->value) % 1000003;
}
echo $total, "\n";
