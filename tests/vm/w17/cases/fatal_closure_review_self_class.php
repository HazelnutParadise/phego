<?php
echo "before\n";
$f = function () { echo self::class; };
$f();
