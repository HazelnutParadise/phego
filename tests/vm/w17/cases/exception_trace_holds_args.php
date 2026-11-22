<?php
// exception_trace_holds_args: objects passed as arguments stay alive through the exception's trace until it is released
class Tracker {
    public $n;

    function __construct($n) {
        $this->n = $n;
    }

    function __destruct() {
        echo "destroy: ", $this->n, "\n";
    }
}

function fail_with($a, $b) {
    echo "in fail_with\n";
    throw new Exception("failed");
}

function plain_fail($x) {
    throw new Exception("plain");
}

function quiet($z) {
    return 1;
}

echo "quiet result: ", quiet(new Tracker("tmp")), "\n";

try {
    fail_with(new Tracker("arg1"), new Tracker("arg2"));
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}
echo "before unset\n";
unset($e);
echo "after unset\n";

$t = new Tracker("kept");
try {
    plain_fail($t);
} catch (Exception $e2) {
    echo "caught2\n";
}
unset($t);
echo "t unset, exception still holds it\n";
unset($e2);
echo "after unset e2\n";
echo "end\n";
