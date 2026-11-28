<?php
function inner() { yield 1; yield 2; }
$i = inner();
function a($i) { yield from $i; }
function b($i) { yield from $i; }
$ga = a($i); $gb = b($i);
echo $ga->current(), "\n";
echo $gb->current(), "\n";
$ga->next();
echo $ga->current(), " ", $gb->current(), "\n";
foreach ($gb as $v) echo "b $v\n";
$c = inner(); $c->next(); $c->next(); $c->next();
function d($c) { $r = yield from $c; echo "d r=", $r === null ? "null" : $r, "\n"; }
foreach (d($c) as $v) echo "d $v\n";
