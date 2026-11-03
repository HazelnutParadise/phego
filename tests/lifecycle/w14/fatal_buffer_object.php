<?php
// REQ-020/REQ-040; T-040/T-079/T-080. New locals still destruct after fatal.
require __DIR__ . '/common.php';
class BufferLocal {
    public function __destruct() { event('destroy:buffer-local'); }
}
ob_start(function ($s) {
    event('flush:object');
    $local = new BufferLocal;
    return $s;
});
echo 'B';
trigger_error('body failure', E_USER_ERROR);
