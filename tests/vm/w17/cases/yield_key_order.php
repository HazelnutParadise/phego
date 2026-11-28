<?php
function k($x) { echo "key $x\n"; return $x; }
function v($x) { echo "value $x\n"; return $x; }
function gen() { yield k('a') => v(1); yield v(2); }
foreach (gen() as $key => $val) echo "$key => $val\n";
