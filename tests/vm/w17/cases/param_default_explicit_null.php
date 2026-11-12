<?php
function f($a = 5)
{
    echo $a === null ? "null" : $a, "\n";
}
f();
f(null);
f(0);
f("");
f(false);
f([]);
function g($a = "x", $b = "y")
{
    echo $a === null ? "n" : $a, $b === null ? "n" : $b, "\n";
}
g(null, null);
g(null);
g("p", null);
$u = null;
g($u);
g($undefined);
g($undefined, $alsoUndefined);
