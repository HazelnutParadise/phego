<?php
// REQ-020; T-039/T-040. The first destructor throwing still unwinds other locals.
require __DIR__ . '/common.php';
class FirstBroken {
    public function __destruct() { event('destroy:local-a'); echo 'D'; throw new Exception('local failure'); }
}
function frame_locals() {
    $a = new FirstBroken;
    $b = new Tracked('local-b');
    echo 'B';
}
try { frame_locals(); } catch (Exception $e) { event('caught'); }
