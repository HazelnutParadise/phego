<?php
echo "before\n";
$f = static fn() => static::$x;
$f();
