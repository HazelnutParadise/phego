<?php
function gen() { global $g; echo "in\n"; try { $g->next(); } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; } yield 1; }
$g = gen();
$g->current();
echo "done\n";
