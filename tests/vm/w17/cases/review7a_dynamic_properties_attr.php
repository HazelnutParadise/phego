<?php
#[AllowDynamicProperties]
class A { }
$a = new A;
$a->x = 1;
echo $a->x, "\n";
