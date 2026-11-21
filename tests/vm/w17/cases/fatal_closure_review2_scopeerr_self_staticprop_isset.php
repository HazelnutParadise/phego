<?php
$f = function () { return isset(self::$x) ? "y" : "n"; };
echo "before\n";
echo $f(), "\n";
