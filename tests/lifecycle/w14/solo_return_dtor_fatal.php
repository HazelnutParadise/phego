<?php
// REQ-040; T-079/T-080. A fatal error in a local's destructor after the
// handler returned bails out the same way.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
class T { public function __destruct() { event("destroy:local"); trigger_error("boom", E_USER_ERROR); } }
register_shutdown_function(function () { event("shutdown:level=" . ob_get_level()); echo "[S]"; });
ob_start(function ($s) { event("flush:outer"); return "<" . $s . ">"; });
ob_start(function ($s) { event("flush:inner"); $t = new T; echo "x"; return "R"; });
echo "body";
ob_end_flush();
echo "|after";
