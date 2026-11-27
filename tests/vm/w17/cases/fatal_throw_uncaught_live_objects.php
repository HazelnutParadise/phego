<?php
// fatal_throw_uncaught_live_objects: Tracker objects alive in globals and locals when an uncaught exception ends the script
class Tracker
{
    public $n;

    function __construct($n)
    {
        $this->n = $n;
    }

    function __destruct()
    {
        echo "destroy: ", $this->n, "\n";
    }
}

$global = new Tracker("global");

function work()
{
    $local = new Tracker("local");
    echo "in work\n";
    throw new Exception("stop");
}

echo "before work\n";
work();
echo "never\n";
