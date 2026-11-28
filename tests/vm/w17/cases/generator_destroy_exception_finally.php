<?php
function gen() { try { yield 1; } finally { throw new Exception("from finally"); } }
$g = gen(); $g->current();
try { unset($g); } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; }
function g2() { try { yield 1; } finally { echo "f2\n"; } }
function thrower() { $g = g2(); $g->current(); throw new Exception("outer"); }
try { thrower(); } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; }
