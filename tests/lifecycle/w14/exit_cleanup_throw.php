<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A destructor throw during exit is observable.
require __DIR__ . '/common.php';
class ExitCleanupThrow { public function __destruct() { event('destroy:throw'); throw new Exception('cleanup failure'); } }
function exit_cleanup_throw() { $a = new ExitCleanupThrow; $b = new Tracked('tail'); exit(7); }
echo 'B';
exit_cleanup_throw();
