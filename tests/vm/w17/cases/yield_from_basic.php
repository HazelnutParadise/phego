<?php
function inner() { yield 1; yield 2; return "ret"; }
function outer() { yield 0; $r = yield from inner(); echo "r=$r\n"; yield 3; yield from [10, 20, 'k' => 30]; yield 4; }
foreach (outer() as $k => $v) echo "$k => $v\n";
