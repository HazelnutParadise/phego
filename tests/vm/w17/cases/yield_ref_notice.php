<?php
function &gen() { $x = 1; yield $x; yield 2; yield f(); $arr = [5]; yield $arr[0]; }
function f() { return 3; }
foreach (gen() as $v) echo $v, "\n";
