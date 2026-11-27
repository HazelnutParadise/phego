<?php
// finally_exit: exit inside try does not run finally; destructors and output before exit still happen; exit status 3
class Tracker
{
    public $n;
    function __construct($n) { $this->n = $n; }
    function __destruct() { echo "destroy: ", $this->n, "\n"; }
}

function work()
{
    $local = new Tracker("local");
    try {
        echo "work try\n";
        exit(3);
    } finally {
        echo "work finally\n";
    }
}

$g = new Tracker("global");
echo "before work\n";
work();
echo "never reached\n";
