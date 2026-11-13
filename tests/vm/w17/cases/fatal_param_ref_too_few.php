<?php
function f(&$a, &$b) { $a = 1; }
echo "start\n";
f($x);
echo "unreached\n";
