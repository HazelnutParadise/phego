<?php
// REQ-020/REQ-022/REQ-040; T-039/T-040/T-043/T-044/T-079/T-080. Local mutation preserves the shared global array.
require __DIR__ . '/common.php';
function local_copy() {
    global $saved;
    $local = ['key' => 'original'];
    $saved = $local;
    $local['key'] = 'changed';
    echo $local['key'];
}
echo 'B';
local_copy();
echo $saved['key'];
