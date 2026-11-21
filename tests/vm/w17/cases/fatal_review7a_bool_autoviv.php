<?php
class A { public ?bool $b = null; public $u = null; }
$a = new A;
$a->u[] = 1;
echo "ok\n";
$a->b['k'] = 1;
