<?php
function gen() { $x = yield 1; echo "got ", $x, "\n"; $y = yield 2; echo "got ", $y, "\n"; return 42; }
$g = gen();
echo $g->current(), "\n";
echo $g->key(), "\n";
echo $g->send("a"), "\n";
echo $g->key(), "\n";
$r = $g->send("b");
echo $r === null ? "null" : $r, "\n";
echo $g->valid() ? "valid" : "done", "\n";
echo $g->getReturn(), "\n";
$g->next();
echo $g->current() === null ? "null" : "x", "\n";
