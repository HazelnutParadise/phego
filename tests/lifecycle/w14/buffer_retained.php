<?php
// REQ-020/REQ-040; T-040/T-079/T-080. No second destructor pass after output handlers.
require __DIR__ . '/common.php';
ob_start(function ($s) {
    event('flush:retained');
    $GLOBALS['late'] = new Tracked('unexpected-late');
    return $s;
});
echo 'B';
