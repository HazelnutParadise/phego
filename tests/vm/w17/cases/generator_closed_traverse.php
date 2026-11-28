<?php
function gen() { yield 1; }
$g = gen(); foreach ($g as $v) echo $v, "\n";
try { foreach ($g as $v) echo $v, "\n"; } catch (Exception $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
$h = gen(); $h->current(); $h->next();
try { foreach ($h as $v) {} } catch (Exception $e) { echo $e->getMessage(), "\n"; }
$k = gen(); $k->current();
foreach ($k as $v) echo "k $v\n";
