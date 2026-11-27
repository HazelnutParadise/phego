<?php
// finally_discarded_return: a finally return replaces a returned Tracker, which is destroyed then; a finally throw replaces it too
class Tracker
{
    public $n;
    function __construct($n) { $this->n = $n; }
    function __destruct() { echo "destroy: ", $this->n, "\n"; }
}

class Stop extends Exception {}

function replaced()
{
    try {
        return new Tracker("from try");
    } finally {
        echo "replaced finally\n";
        return "from finally";
    }
}
$r = replaced();
echo "replaced result: ", $r, "\n";

function thrownOver()
{
    try {
        return new Tracker("discarded by throw");
    } finally {
        echo "thrownOver finally\n";
        throw new Stop("finally throw");
    }
}
try {
    $x = thrownOver();
    echo "not reached\n";
} catch (Stop $e) {
    echo "caught: ", $e->getMessage(), "\n";
}
echo "after catch\n";

function twoTrackers()
{
    try {
        return new Tracker("try-one");
    } finally {
        return new Tracker("finally-one");
    }
}
$t = twoTrackers();
echo "kept: ", $t->n, "\n";
$t = null;
echo "done\n";
