<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Original test fixture.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
class Tracked {
    public function __construct(public string $name) {}
    public function __destruct() { event("destroy:" . $this->name); echo "D"; }
}
$a = new Tracked("global");
register_shutdown_function(function () { event("shutdown:1"); echo "S"; });
register_shutdown_function(function () { event("shutdown:2"); });
ob_start(function ($s) { event("flush:outer"); return "[" . $s . "]"; });
ob_start(function ($s) { event("flush:inner"); return "(" . $s . ")"; });
