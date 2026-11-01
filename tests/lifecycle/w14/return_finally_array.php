<?php
// REQ-020/REQ-022; T-039/T-040/T-043/T-044. A pending by-value return owns its array.
require __DIR__ . '/common.php';
function pending_array() {
    $GLOBALS['return_value'] = ['key' => 'kept'];
    try { return $GLOBALS['return_value']; }
    finally { unset($GLOBALS['return_value']); event('finally'); }
}
echo 'B';
$result = pending_array();
echo $result['key'];
event('returned');
