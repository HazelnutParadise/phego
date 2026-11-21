<?php
$f = function () { return 1; };
echo "a\n";
echo isset($f->x) ? "set" : "unset", "\n";
echo $f->x ?? "dflt", "\n";
echo $f->x, "|\n";
$f->x = 1;
