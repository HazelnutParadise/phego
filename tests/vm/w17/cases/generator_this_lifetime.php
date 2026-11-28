<?php
class A { function __destruct() { echo "~A\n"; } function gen() { yield 1; yield 2; } }
$g = (new A)->gen();
echo "created\n";
foreach ($g as $v) echo $v, "\n";
echo "loop done\n";
unset($g);
echo "end\n";
