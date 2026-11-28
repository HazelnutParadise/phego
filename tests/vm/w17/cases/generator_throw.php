<?php
function gen() { try { yield 1; } catch (Exception $e) { echo "caught in gen: ", $e->getMessage(), "\n"; yield 99; } finally { echo "finally\n"; } yield 3; }
$g = gen();
echo $g->current(), "\n";
echo $g->throw(new Exception("x")), "\n";
$g->next();
echo $g->current(), "\n";
$g->next();
echo $g->valid() ? "v" : "nv", "\n";
$h = gen();
try { $h->throw(new Exception("before start")); } catch (Exception $e) { echo "out: ", $e->getMessage(), "\n"; }
echo $h->valid() ? "v" : "nv", "\n";
function g2() { yield 1; }
$k = g2(); $k->current();
try { $k->throw(new LogicException("uncaught")); } catch (Exception $e) { echo $e::class, " ", $e->getMessage(), "\n"; }
echo $k->valid() ? "v" : "nv", "\n";
