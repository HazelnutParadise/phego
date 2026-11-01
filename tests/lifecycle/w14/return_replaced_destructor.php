<?php
// REQ-020; T-039/T-040. A destructor exception consumes a pending return before catch.
require __DIR__ . '/common.php';
class ReturnTracked {
    public function __destruct() { event('destroy:returned'); echo 'R'; }
}
class UnwindFailure {
    public function __destruct() { event('destroy:local'); throw new Exception('local failure'); }
}
function pending_object() { $returned = new ReturnTracked; $local = new UnwindFailure; return $returned; }
echo 'B';
try { $result = pending_object(); } catch (Exception $e) { event('caught'); }
