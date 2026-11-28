<?php
function gen() { echo "run\n"; yield 1; yield 2; }
$g = gen();
$g->rewind();
$g->rewind();
echo $g->current(), "\n";
$g->next();
try { $g->rewind(); } catch (Exception $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
foreach ($g as $v) { echo $v, "\n"; }
