<?php
$f = function (int $a) { return 1; };
echo "x\n";
$f->__invoke([]);
