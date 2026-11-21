<?php
class A { public ?int $i = null; public ?string $s = null; }
$x = new A;
$x->s['k'] = 1;
echo "after\n";
