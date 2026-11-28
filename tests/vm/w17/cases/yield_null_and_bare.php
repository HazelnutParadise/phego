<?php
function gen() { yield; yield null; $x = yield; echo "x=", $x === null ? "null" : $x, "\n"; }
foreach (gen() as $k => $v) echo $k, " ", $v === null ? "null" : $v, "\n";
