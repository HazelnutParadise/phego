<?php
class A { protected $p = "A"; }
class B extends A { protected $p = "B"; }
class C extends A { function set($o) { $o->p = "set"; return $o->p; } function un($o) { unset($o->p); return isset($o->p) ? "y" : "n"; } }
$c = new C; echo $c->set(new B), $c->un(new B), "\n";
