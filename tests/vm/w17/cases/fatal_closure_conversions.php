<?php
$f = function () { return 1; };
echo (int)$f, "\n";
echo $f == 1 ? "eq" : "ne", "\n";
echo $f == true ? "eq" : "ne", "\n";
echo $f == null ? "eq" : "ne", "\n";
$g = function () { return 1; };
echo $f < $g ? "lt" : "nlt", $f > $g ? "gt" : "ngt", $f <=> $g, "\n";
foreach ($f as $k => $v) { echo "prop\n"; }
unset($f->y);
echo "done\n";
echo $f . "x";
