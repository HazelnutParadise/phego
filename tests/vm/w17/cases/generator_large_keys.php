<?php
function gen() { yield PHP_INT_MAX => 'a'; yield 'b'; }
try { foreach (gen() as $k => $v) echo $k, " ", $v, "\n"; } catch (Error $e) { echo $e::class, "\n"; }
function g2() { yield -5 => 'a'; yield 'b'; yield '10' => 'c'; yield 'd'; }
foreach (g2() as $k => $v) echo $k, " ", $v, "\n";
