<?php
// REQ-040; T-079/T-080. A local destructor that exits after the handler
// returned keeps the returned string and the deprecation; shutdown
// functions then run.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
class T { public function __destruct() { event("destroy:local"); exit(4); } }
register_shutdown_function(function () { event("shutdown:solo"); echo "S"; });
ob_start(function ($s) { event("flush:outer"); return "<" . $s . ">"; });
ob_start(function ($s) { event("flush:inner"); $t = new T; echo "x"; return "R"; });
echo "body";
ob_end_flush();
echo "after";
