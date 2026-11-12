<?php
function f($label, $value)
{
    echo "[", $label, "]";
    return $value;
}
$r = f("a", false) && f("b", true);
echo " =", $r ? 1 : 0, "\n";
$r = f("a", true) && f("b", false);
echo " =", $r ? 1 : 0, "\n";
$r = f("a", true) || f("b", true);
echo " =", $r ? 1 : 0, "\n";
$r = f("a", 0) || f("b", 0) || f("c", 1);
echo " =", $r ? 1 : 0, "\n";
$r = f("a", 1) xor f("b", 1);
echo " =", $r ? 1 : 0, "\n";
$r = f("a", 0) ? f("b", 1) : f("c", 2);
echo " =", $r, "\n";
$r = f("a", "") ?: f("b", "fallback");
echo " =", $r, "\n";
$r = f("a", "keep") ?: f("b", "x");
echo " =", $r, "\n";
$n = null;
$r = $n ?? f("b", "def");
echo " =", $r, "\n";
$m = 0;
$r = $m ?? f("b", "unused");
echo " =", $r, "\n";
$r = f("l", 1) + f("r", 2) * f("m", 3);
echo " =", $r, "\n";
