<?php
function f(...$a) { foreach ($a as $k => $v) echo "$k=$v "; echo "\n"; }
function g() { yield 1; yield 2; yield 'n' => 3; }
f(...g());
function h() { yield 'n' => 1; yield 2; }
try { f(...h()); } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
function k() { yield 1.5 => 1; }
try { f(...k()); } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
