<?php
echo "before\n";
$f = function () { return self::foo(); };
$f();
