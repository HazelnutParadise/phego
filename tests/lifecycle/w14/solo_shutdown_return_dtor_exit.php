<?php
// REQ-040; T-079/T-080. At request shutdown the same exit still bails out
// of the flush, so nothing is printed and the exit status is kept.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
class T { public function __destruct() { event("destroy:local"); exit(4); } }
ob_start(function ($s) { event("flush:outer"); return "<" . $s . ">"; });
ob_start(function ($s) { event("flush:inner"); $t = new T; echo "x"; return "R"; });
echo "body";
