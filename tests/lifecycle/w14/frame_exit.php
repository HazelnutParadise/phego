<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Exit unwinds function locals before shutdown.
require __DIR__ . '/common.php';
function frame_exit() { $local = new Tracked('local'); try { exit(7); } finally { event('finally'); } }
echo 'B';
frame_exit();
