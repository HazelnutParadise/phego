<?php
class A { public int $i = 1; }
$a = new A;
$s = "str";
$a->i = &$s;
