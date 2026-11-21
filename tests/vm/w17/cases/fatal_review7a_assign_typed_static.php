<?php
class A { public static int $i = 1; }
$s = "str";
A::$i = &$s;
