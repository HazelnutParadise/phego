<?php
function gen() { yield 1; }
try { $g = new Generator; } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
$g = gen();
try { $h = clone $g; } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
