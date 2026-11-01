<?php
// REQ-020; T-039/T-040. By-value return snapshots the value before finally changes a reference.
require __DIR__ . '/common.php';
function pending_reference() {
    $a = ['key' => 'kept'];
    $alias =& $a;
    try { return $a; }
    finally { $alias = ['key' => 'changed']; event('finally'); }
}
echo 'B';
$result = pending_reference();
echo $result['key'];
event('returned');
