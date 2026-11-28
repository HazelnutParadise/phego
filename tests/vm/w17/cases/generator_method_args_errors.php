<?php
function gen() { yield 1; }
$g = gen();
try { $g->send(); } catch (ArgumentCountError $e) { echo $e->getMessage(), "\n"; }
try { $g->throw("x"); } catch (TypeError $e) { echo $e->getMessage(), "\n"; }
try { $g->current(1); } catch (ArgumentCountError $e) { echo $e->getMessage(), "\n"; }
echo $g->send(value: 5), "\n";
