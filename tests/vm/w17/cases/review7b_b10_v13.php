<?php
class A { protected int $p = 1; }
class B extends A { protected int $p = 2; }
class C extends A { function get($o) { $o->p++; return $o->p; } }
echo (new C)->get(new B), "\n";
