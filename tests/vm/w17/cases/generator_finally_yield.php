<?php
function gen() { try { yield 1; } finally { echo "fin\n"; yield 2; echo "after fin yield\n"; } }
$g = gen(); $g->current();
try { unset($g); } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
echo "done\n";
