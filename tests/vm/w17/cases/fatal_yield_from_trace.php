<?php
function inner() { yield 1; throw new Exception("in"); }
function middle() { yield from inner(); }
function outer() { yield from middle(); }
$g = outer();
foreach ($g as $v) { echo $v, "\n"; }
