<?php
function gen() { global $g; yield 1; foreach ($g as $v) { echo "inner $v\n"; } yield 2; }
$g = gen();
try { foreach ($g as $v) echo "outer $v\n"; } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
