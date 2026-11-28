<?php
class Box { private $items = ['a', 'b']; function each() { return function () { foreach ($this->items as $i) yield $i; }; } }
$f = (new Box)->each();
foreach ($f() as $v) echo $v, "\n";
class P { protected $x = 5; function gen() { yield $this->x; yield static::class; } }
class Q extends P { protected $x = 6; }
foreach ((new Q)->gen() as $v) echo $v, "\n";
