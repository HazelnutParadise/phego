<?php
$t = "abc";
$z = "0x";
foreach ([1.5, true, false, null, "1x", "x", " 1", "1 ", -1, -4, 3, "2", "-1", 1e3] as $k) {
    echo empty($t[$k]) ? "E" : "n", isset($t[$k]) ? "S" : "u", " ";
}
echo "\n";
echo empty($z[0]) ? "e" : "n", empty($z[1]) ? "e" : "n", empty($z[2]) ? "e" : "n", empty($z[-1]) ? "e" : "n", empty($z[-2]) ? "e" : "n", "\n";
echo empty($t[0][0]) ? "e" : "n", empty($t[1][5]) ? "e" : "n", empty($t[5][0]) ? "e" : "n", "\n";
echo empty("abc"[1]) ? "e" : "n", empty("0"[0]) ? "e" : "n", empty(""[0]) ? "e" : "n", "\n";
$u = "0";
echo empty($u[0]) ? "e" : "n", empty($u["0"]) ? "e" : "n", empty($u[0.0]) ? "e" : "n", "\n";
echo "done\n";
