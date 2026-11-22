<?php
// try_catch_reference_variable: the catch variable is a reference to another variable, a global, or an array element; what that storage holds afterwards
class Oops extends Exception {}

function fire($m)
{
    throw new Oops($m);
}

$other = "untouched";
$e = &$other;
try {
    fire("ref one");
} catch (Oops $e) {
    echo "1 inside: ", $e->getMessage(), "\n";
}
echo "1 other is exception: ", $other instanceof Oops ? "yes" : "no", "\n";
echo "1 other message: ", $other instanceof Oops ? $other->getMessage() : "none", "\n";

$gv = "start";
function viaGlobal()
{
    global $gv;
    try {
        fire("via global");
    } catch (Oops $gv) {
        echo "2 inside function: ", $gv->getMessage(), "\n";
    }
}
viaGlobal();
echo "2 global holds exception: ", $gv instanceof Oops ? "yes" : "no", "\n";

$arr = ['k' => 'before'];
$r = &$arr['k'];
try {
    fire("array element");
} catch (Oops $r) {
    echo "3 inside element: ", $r->getMessage(), "\n";
}
echo "3 array k is exception: ", $arr['k'] instanceof Oops ? "yes" : "no", "\n";

$plain = "p0";
try {
    fire("plain");
} catch (Oops $plain) {
    echo "4 plain caught: ", $plain->getMessage(), "\n";
}
echo "4 plain now exception: ", $plain instanceof Oops ? "yes" : "no", "\n";
