<?php
function f($a = "A", $b = "B", $c = "C")
{
    echo $a, $b, $c, "\n";
}
f();
f(1);
f(1, 2);
f(1, 2, 3);
f(1, 2, 3, 4);
function g($req, $a = "A", $b = "B")
{
    echo $req, $a, $b, "\n";
}
g(0);
g(0, 1);
g(0, 1, 2);
