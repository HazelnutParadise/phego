<?php
function f($a = 1 + 2, $b = "a" . "b", $c = 2 * 3 + 1, $d = -5, $e = !true, $f = 1 << 3)
{
    echo $a, "|", $b, "|", $c, "|", $d, "|", $e === false ? "false" : "?", "|", $f, "\n";
}
f();
function g($a = 10 % 3, $b = 7 / 2, $c = "5" + 3, $d = 2 ** 10, $e = 1 <=> 2, $f = 5 > 3)
{
    echo $a, "|", $b, "|", $c, "|", $d, "|", $e, "|", $f, "\n";
}
g();
function h($a = null ?? "dflt", $b = 1 ? "yes" : "no", $c = true && false, $d = 6 & 3 | 8, $e = 1 . 2, $f = "x" . 1 + 2)
{
    echo $a, "|", $b, "|", $c === false ? "false" : "?", "|", $d, "|", $e, "|", $f, "\n";
}
h();
function k($a = [1, 2, 3][1], $b = [1 + 1, "a" . "b"], $c = (int) "12abc", $d = "abc"[1], $e = 3 === 3.0)
{
    echo $a, "|", $b[0], $b[1], "|", $c, "|", $d, "|", $e === false ? "false" : "?", "\n";
}
k();
