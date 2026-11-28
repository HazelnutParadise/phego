<?php
function a(): Generator { yield 1; }
function b(): iterable { yield 2; }
function c(): Traversable { yield 3; }
function d(): Iterator { yield 4; }
function e(): mixed { yield 5; }
function f(): object { yield 6; }
foreach ([a(), b(), c(), d(), e(), f()] as $g) foreach ($g as $v) echo $v;
echo "\n";
function r(): Generator { yield 1; return "x"; }
$g = r(); foreach ($g as $v) {} echo $g->getReturn(), "\n";
