<?php
// try_catch_variable_scope: the catch variable stays set after the block, overwrites an existing variable, and is unset when nothing was thrown
class Tracker
{
    public $n;
    function __construct($n) { $this->n = $n; }
    function __destruct() { echo "destroy: ", $this->n, "\n"; }
}

class Fail extends Exception {}

function boom($m)
{
    throw new Fail($m);
}

echo "1 start\n";
try {
    boom("first");
} catch (Fail $e) {
    echo "1 caught: ", $e->getMessage(), "\n";
}
echo "1 after block, e still set: ", isset($e) ? "yes" : "no", "\n";
echo "1 e message: ", $e->getMessage(), "\n";

echo "2 nothing thrown\n";
try {
    echo "2 try ok\n";
} catch (Fail $g) {
    echo "never\n";
}
echo "2 g isset: ", isset($g) ? "yes" : "no", "\n";

echo "3 overwrite\n";
$e = new Tracker("old-value");
echo "3 holding tracker\n";
try {
    boom("second");
} catch (Fail $e) {
    echo "3 caught into e: ", $e->getMessage(), "\n";
}
echo "3 after overwrite\n";

echo "4 unset\n";
unset($e);
echo "4 e isset: ", isset($e) ? "yes" : "no", "\n";
echo "end\n";
