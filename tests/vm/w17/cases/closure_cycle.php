<?php
// closure_cycle: a closure that captures $this inside its own object forms a cycle
class Cyc {
    public $name;
    public $cb;

    public function __construct($name) {
        $this->name = $name;
        $this->cb = function () {
            return "cb of " . $this->name;
        };
        echo "construct ", $name, "\n";
    }
    public function __destruct() {
        echo "destruct ", $this->name, "\n";
    }
}

class Plain {
    public $name;

    public function __construct($name) {
        $this->name = $name;
        echo "construct ", $name, "\n";
    }
    public function __destruct() {
        echo "destruct ", $this->name, "\n";
    }
}

$cyc = new Cyc("cycle");
echo "cb: ", ($cyc->cb)(), "\n";
unset($cyc);
echo "after unset cycle (no destruct expected here)\n";

$plain = new Plain("plain");
unset($plain);
echo "after unset plain (destruct above)\n";

$keep = new Cyc("kept");
$keep->cb = null;
unset($keep);
echo "after breaking the cycle first (destruct above)\n";

echo "end of script\n";
