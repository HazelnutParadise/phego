<?php
function inner() { throw new Exception("deep"); }
function gen($a) { yield 1; inner(); yield 2; }
foreach (gen(5) as $v) { echo $v, "\n"; }
