<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A destructor may replace the pending exit code.
require __DIR__ . '/common.php';
class ExitCleanupExit { public function __destruct() { event('destroy:exit'); exit(9); } }
function exit_cleanup_exit() { $a = new ExitCleanupExit; $b = new Tracked('tail'); exit(7); }
echo 'B';
exit_cleanup_exit();
