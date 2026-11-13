<?php
function f($i = 42, $f = 1.5, $s = "text", $b = true, $n = null, $neg = -7)
{
    echo $i, "|", $f, "|", $s, "|", $b, "|", $n === null ? "null" : "notnull", "|", $neg, "\n";
}
f();
f(1);
f(1, 2);
f(1, 2, 3);
f(1, 2, 3, false);
f(1, 2, 3, false, "x");
f(1, 2, 3, false, "x", 0);
function g($a = false, $b = 0, $c = "", $d = 0.0, $e = "0", $f = -0.0)
{
    echo $a === false ? "F" : "?", $b === 0 ? "0" : "?", $c === "" ? "E" : "?", $d === 0.0 ? "D" : "?", $e === "0" ? "Z" : "?", $f, "\n";
}
g();
