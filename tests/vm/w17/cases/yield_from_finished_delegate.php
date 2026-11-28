<?php
function inner() { yield 1; return 7; }
$i = inner();
function outer($i) { $r = yield from $i; echo "r=$r\n"; yield 2; }
$o = outer($i);
foreach ($o as $v) echo $v, "\n";
$o2 = outer($i);
try { foreach ($o2 as $v) echo $v, "\n"; } catch (Error $e) { echo $e->getMessage(), "\n"; }
