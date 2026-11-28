<?php
function gen() { $a = yield; echo "a=", var_s($a), "\n"; $b = (yield 1) + 10; echo "b=$b\n"; echo "c=", yield 'k' => 'v', "\n"; }
function var_s($v) { return $v === null ? "null" : $v; }
$g = gen();
echo var_s($g->current()), "|", var_s($g->key()), "\n";
$g->send("first");
echo $g->current(), "|", $g->key(), "\n";
$g->send(5);
echo $g->current(), "|", $g->key(), "\n";
$g->send("third");
echo $g->valid() ? "v" : "nv", "\n";
