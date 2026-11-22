<?php
// finally_return_object_lifetime: a Tracker returned from try survives a finally that unsets the local; the caller decides when it dies
class Tracker
{
    public $n;
    function __construct($n) { $this->n = $n; }
    function __destruct() { echo "destroy: ", $this->n, "\n"; }
}

function make($n)
{
    $t = new Tracker($n);
    try {
        echo "make try ", $n, "\n";
        return $t;
    } finally {
        unset($t);
        echo "make finally after unset\n";
    }
}

echo "marker 1\n";
$kept = make("kept");
echo "marker 2: caller holds kept\n";

make("discard");
echo "marker 3: discarded\n";

$tmp = make("used")->n;
echo "marker 4: used property ", $tmp, "\n";

$kept = null;
echo "marker 5: kept released\n";
echo "end\n";
