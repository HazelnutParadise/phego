<?php
function g() { global $x; yield from $x; }
$x = g();
try { foreach ($x as $v) {} } catch (Error $e) { echo $e->getMessage(), "\n"; }
function h() { yield from 5; }
try { foreach (h() as $v) {} } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
