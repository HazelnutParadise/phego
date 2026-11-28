<?php
function inner() { yield 1; throw new Exception("inner died"); }
$i = inner();
function outer($i) { try { yield from $i; } catch (Exception $e) { echo "outer: ", $e->getMessage(), "\n"; } yield 5; }
$a = outer($i); $b = outer($i);
echo $a->current(), " ", $b->current(), "\n";
try { $a->next(); } catch (Exception $e) { echo "a: ", $e->getMessage(), "\n"; }
echo $a->current(), "\n";
try { echo $b->current(), "\n"; } catch (Throwable $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
