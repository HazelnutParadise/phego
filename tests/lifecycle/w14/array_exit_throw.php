<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A throw during exit cleanup must not resume the caller.
require __DIR__ . '/common.php';
class FirstExit { public function __destruct() { event('destroy:exit'); exit(7); } }
class AfterExitThrow { public function __destruct() { event('destroy:throw'); throw new Exception('cleanup failure'); } }
function release_exit_throw() { $locals = [new FirstExit, new AfterExitThrow, new Tracked('tail')]; }
echo 'B';
try { release_exit_throw(); } catch (Exception $e) { event('caught'); echo 'unexpected'; }
