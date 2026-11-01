<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080.
require __DIR__ . '/common.php';
function run_frame() {
    $local = new Tracked("local");
    try { echo "B"; throw new Exception("caught"); }
    finally { event("finally"); }
}
try { run_frame(); } catch (Exception $e) { event("caught"); }
