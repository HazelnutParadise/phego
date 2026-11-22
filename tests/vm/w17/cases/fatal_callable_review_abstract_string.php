<?php
interface I { function m(); }
echo "before\n";
$f = "I::m";
$f();
