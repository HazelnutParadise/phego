<?php
echo "before\n";
$f = function () { return new static; };
$f();
