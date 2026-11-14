<?php
echo empty($u) ? "e" : "n", "\n";
echo empty($u[0]) ? "e" : "n", empty($u[0][1]) ? "e" : "n", empty($u["k"]["j"]["i"]) ? "e" : "n", "\n";
$n = null;
$i = 5;
$b = true;
$f = false;
$fl = 1.5;
echo empty($n[0]) ? "e" : "n", empty($i[0]) ? "e" : "n", empty($b[0]) ? "e" : "n", empty($f[0]) ? "e" : "n", empty($fl[0]) ? "e" : "n", "\n";
echo empty($i[0][1]) ? "e" : "n", "\n";
$a = ["x" => ["y" => "0", "z" => "1", "w" => null, "v" => [], "u" => [0]]];
echo empty($a["x"]["y"]) ? "e" : "n", empty($a["x"]["z"]) ? "e" : "n", empty($a["x"]["w"]) ? "e" : "n", empty($a["x"]["v"]) ? "e" : "n", empty($a["x"]["u"]) ? "e" : "n", "\n";
echo empty($a["x"]["missing"]) ? "e" : "n", empty($a["q"]["y"]) ? "e" : "n", empty($a["x"]["z"][0]) ? "e" : "n", empty($a["x"]["u"][0]) ? "e" : "n", "\n";
echo empty($a["x"]["z"]["deeper"]) ? "e" : "n", "\n";
echo empty($a[$u2]) ? "e" : "n", "\n";
echo "after\n";
echo isset($u) ? "s" : "u", empty($u) ? "e" : "n", "\n";
