<?php
$s = "0";
echo $s[0] ? "truthy" : "falsy", "\n";
echo $s[0] == 0 ? "eq0" : "ne0", $s[0] === "0" ? "same" : "diff", "\n";
$nl = "a\nb";
echo "[", $nl[1], "]", "\n";
$sp = " x ";
echo "[", $sp[0], $sp[1], $sp[2], "]", "\n";
$tab = "\t";
echo "[", $tab[0], "]", "\n";
$q = "'\"";
echo $q[0], $q[1], "\n";
$d = '$a';
echo $d[0], $d[1], "\n";
$bs = 'a\\b';
echo $bs[1], $bs[2], "\n";
$e = "";
echo isset($e[0]) ? "y" : "n", $e == "" ? "empty" : "not", "\n";
