<?php
$f = function ($o) { return $o instanceof self ? "yes" : "no"; };
echo $f(1), $f(null), $f("s"), "\n";
$g = fn($o) => $o instanceof static ? "yes" : "no";
echo $g([]), "\n";
