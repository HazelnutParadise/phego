<?php
// REQ-020; T-039/T-040. Frame locals are released in variable order.
require __DIR__ . '/common.php';
function frame_locals() {
    $a = new Tracked('local-a');
    $b = new Tracked('local-b');
    echo 'B';
}
frame_locals();
event('returned');
