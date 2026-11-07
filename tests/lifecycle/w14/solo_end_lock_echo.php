<?php
// REQ-040; T-079/T-080. A handler that echoes and then hits the output lock
// error never returns, so PHP 8.5.12 reports only the fatal error, not the
// handler-output deprecation.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
register_shutdown_function(function () { event("shutdown:solo"); echo "[S]"; });
ob_start(function ($s) { event("flush:lock"); echo "x"; ob_end_flush(); return $s; });
echo "body";
ob_end_flush();
