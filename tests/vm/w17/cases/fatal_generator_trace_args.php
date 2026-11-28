<?php
class A { function gen($x, $y) { yield 1; throw new RuntimeException("bad"); } }
foreach ((new A)->gen([1, 2], "str") as $v) { echo $v, "\n"; }
