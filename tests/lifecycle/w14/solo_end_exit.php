<?php
// REQ-040; T-079/T-080. exit in a handler flushed from PHP code passes the
// buffer and the handler's echo to the next level without the deprecation,
// as a throw does; shutdown functions then run.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
register_shutdown_function(function () { event("shutdown:solo"); echo "S"; });
ob_start(function ($s) { event("flush:outer"); return "[" . $s . "]"; });
ob_start(function ($s) { event("flush:exit"); echo "x"; exit(3); });
echo "body";
ob_end_flush();
echo "after";
