<?php
$f = function () { return isset(static::$x) ? "y" : "n"; };
$g = function () { return empty(parent::$x) ? "y" : "n"; };
echo "before\n";
echo $f(), "\n";
