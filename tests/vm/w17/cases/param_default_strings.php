<?php
function f($a = "single 'quoted'", $b = 'dollar $notvar', $c = "tab\there", $d = "nl\n", $e = 'back\\slash', $f = "")
{
    echo $a, "|", $b, "|", $c, "|", $d, $e, "|", $f, "|\n";
}
f();
function g($a = "é", $b = "日本語", $c = "a" . "\x41" . "\u{42}")
{
    echo $a, $b, $c, "\n";
}
g();
function h($s = "abc")
{
    return $s . "-" . $s;
}
echo h(), h("z"), "\n";
