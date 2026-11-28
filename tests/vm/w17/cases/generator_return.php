<?php
function gen() { yield 1; return 2; }
$g = gen();
try { $g->getReturn(); } catch (Exception $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
foreach ($g as $v) echo $v, "\n";
echo $g->getReturn(), "\n";
function g2() { return 5; yield; }
$h = g2();
echo $h->valid() ? "v" : "nv", "\n";
echo $h->getReturn(), "\n";
function g3() { throw new Exception("boom"); yield; }
$t = g3();
try { $t->current(); } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; }
try { $t->getReturn(); } catch (Exception $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
echo $t->valid() ? "v" : "nv", "\n";
