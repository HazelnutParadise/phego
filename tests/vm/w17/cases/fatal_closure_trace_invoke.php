<?php
$f = function ($a) { return nope($a); };
echo "x\n";
$f->__invoke("s");
