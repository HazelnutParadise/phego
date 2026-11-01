<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Exit survives an earlier aggregate exception.
require __DIR__ . '/common.php';
class EarlierExitThrow { public function __destruct() { event('destroy:throw'); throw new Exception('earlier throw'); } }
class LaterExit { public function __destruct() { event('destroy:exit'); exit(7); } }
function release_throw_exit() { $locals = [new EarlierExitThrow, new LaterExit, new Tracked('tail')]; }
echo 'B';
try { release_throw_exit(); } catch (Exception $e) { event('caught'); echo 'unexpected'; }
