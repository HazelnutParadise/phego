<?php
// REQ-040; T-079/T-080. A fatal error inside a handler bails out before its
// buffer leaves the stack, so PHP 8.5.12 still counts it in shutdown
// functions and the request-shutdown flush hits the output lock error.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
register_shutdown_function(function () { event("shutdown:level=" . ob_get_level()); echo "[S]"; });
ob_start(function ($s) { event("flush:outer"); return "<" . $s . ">"; });
ob_start(function ($s) { event("flush:inner"); echo "x"; trigger_error("boom", E_USER_ERROR); return "R"; });
echo "body";
ob_end_flush();
echo "|after";
