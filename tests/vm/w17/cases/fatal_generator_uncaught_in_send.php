<?php
function gen() { $x = yield 1; throw new RuntimeException("got $x"); }
$g = gen(); $g->current();
$g->send("v");
