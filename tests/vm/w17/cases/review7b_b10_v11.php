<?php
class A { protected $p = "A"; }
class B extends A { protected $p = "B"; }
class C extends A { function get($o) { $r = &$o->p; $r = "R"; return $o->p; } function ii($o) { return isset($o->p) ? "y" : "n"; } }
$c = new C; echo $c->get(new B), $c->ii(new B), "\n";
