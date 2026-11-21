<?php
namespace N;
function f() { return 1; }
$g = f(...); $h = \N\f(...);
echo $g(), $h(), $g == $h ? "eq" : "ne", "\n";
$c = function () { return __FUNCTION__ . "|" . __NAMESPACE__; };
echo $c(), "\n";
class K { function m() { return fn() => __METHOD__ . "|" . __CLASS__ . "|" . __FUNCTION__; } }
echo (new K)->m()(), "\n";
echo ($c instanceof \Closure) ? "y" : "n", "\n";
nope(...);
