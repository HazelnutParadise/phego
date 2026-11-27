<?php
// throw_deep_unwind: 300 nested recursive calls then a throw; frames below depth 3 hold a Tracker, the rest do not
class Tracker {
    public $n;

    function __construct($n) {
        $this->n = $n;
    }

    function __destruct() {
        echo "destroy: ", $this->n, "\n";
    }
}

$calls = 0;

function rec($d) {
    global $calls;
    $calls = $calls + 1;
    if ($d == 300) {
        throw new Exception("bottom at " . $d);
    }
    if ($d < 3) {
        $t = new Tracker("lvl" . $d);
        rec($d + 1);
    } else {
        rec($d + 1);
    }
}

try {
    rec(0);
    echo "not reached\n";
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}
echo "calls: ", $calls, "\n";
echo "exception_line_ok: ", ($e->getLine() > 0 ? 1 : 0), "\n";
echo "end\n";
