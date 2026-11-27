<?php
// finally_variable_state: variables assigned in try before a throw, in catch and in finally are all visible after the statement
class Fault extends Exception {}

$a = "a0";
$b = "b0";
$c = "c0";
try {
    $a = "a1";
    throw new Fault("mid");
    $b = "b1";
} catch (Fault $e) {
    $b = "b2";
    echo "in catch: a=", $a, " b=", $b, "\n";
} finally {
    $c = "c1";
}
echo "after: a=", $a, " b=", $b, " c=", $c, "\n";
echo "catch variable message: ", $e->getMessage(), "\n";

function stateFn()
{
    $n = 0;
    try {
        $n = $n + 1;
        throw new Fault("in fn");
    } catch (Fault $e) {
        $n = $n + 10;
    } finally {
        $n = $n + 100;
        echo "finally n=", $n, "\n";
    }
    return $n;
}
$res = stateFn();
echo "stateFn returns: ", $res, "\n";

$z = "z0";
try {
    try {
        $z = "z1";
        throw new Fault("escape");
    } finally {
        $z = $z . "+f";
    }
} catch (Fault $e) {
    echo "z after escape: ", $z, "\n";
}
