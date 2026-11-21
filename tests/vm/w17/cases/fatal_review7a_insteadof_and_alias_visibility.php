<?php
trait A { function hi() { return "A"; } }
trait B { function hi() { return "B"; } }
class C { use A, B { A::hi insteadof B; B::hi as protected bhi; hi as private phi; } function t() { return $this->bhi() . $this->phi(); } }
$c = new C;
echo $c->hi(), $c->t(), "\n";
$c->bhi();
