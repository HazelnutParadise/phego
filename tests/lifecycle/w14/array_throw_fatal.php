<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A later fatal cannot be caught as an earlier array exception.
require __DIR__ . '/common.php';
class EarlierThrow {
    public function __destruct() { event('destroy:throw'); throw new Exception('earlier throw'); }
}
class LaterFatal {
    public function __destruct() { event('destroy:fatal'); trigger_error('fatal destructor', E_USER_ERROR); }
}
function release_throw_fatal() { $locals = [new EarlierThrow, new LaterFatal, new Tracked('tail')]; }
echo 'B';
try { release_throw_fatal(); } catch (Exception $e) { event('caught'); echo 'unexpected'; }
