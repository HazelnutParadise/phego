<?php
class A { public $id; function __construct($id) { $this->id = $id; } function __call($n, $a) { $o = "c:" . $this->id . ":" . $n; foreach ($a as $v) { $o .= "," . $v; } return $o; } }
$f = (new A("one"))->zz(...);
echo $f(1), "\n";
echo $f->call(new A("two"), 2, 3), "\n";
echo $f(4), "\n";
