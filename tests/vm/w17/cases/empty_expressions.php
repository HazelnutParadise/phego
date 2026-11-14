<?php
function f($v)
{
    echo "[f]";
    return $v;
}
echo empty(f(0)) ? "e" : "n", empty(f("x")) ? "e" : "n", "\n";
echo empty(1 - 1) ? "e" : "n", empty("a" . "") ? "e" : "n", empty([] + []) ? "e" : "n", empty(1 + 1) ? "e" : "n", "\n";
echo empty(empty(0)) ? "e" : "n", empty(!empty(0)) ? "e" : "n", "\n";
$x = 0;
echo empty($x ?? 1) ? "e" : "n", empty($y ?? 0) ? "e" : "n", empty($x ?: "fallback") ? "e" : "n", "\n";
echo empty(PHP_EOL) ? "e" : "n", empty(E_ALL - E_ALL) ? "e" : "n", "\n";
echo empty($x = 5) ? "e" : "n", $x, "\n";
echo empty($x++) ? "e" : "n", $x, "\n";
echo empty(match ($x) { 6 => 0, default => 1 }) ? "e" : "n", "\n";
echo empty(@$undef) ? "e" : "n", "\n";
