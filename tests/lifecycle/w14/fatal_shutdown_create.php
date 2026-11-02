<?php
// REQ-020/REQ-040; T-039/T-040/T-080. Fatal marks old objects, not future ones.
require __DIR__ . '/common.php';
register_shutdown_function(function () {
    event('shutdown:create');
    unset($GLOBALS['a']);
    $local = new Tracked('new-local');
    unset($local);
    $GLOBALS['new'] = new Tracked('new-retained');
});
echo 'B';
trigger_error('body failure', E_USER_ERROR);
