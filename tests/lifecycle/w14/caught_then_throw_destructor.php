<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A handled throw is not an implicit previous exception.
require __DIR__ . '/common.php';
class ThrowSecond { public function __destruct() { throw new Exception('second'); } }
function caught_then_throw() {
    try { throw new Exception('first'); } catch (Exception $e) { event('caught:first'); }
    $v = new ThrowSecond; unset($v);
}
echo 'B';
try { caught_then_throw(); } catch (Exception $e) {
    event('caught:' . $e->getMessage());
    for ($p = $e->getPrevious(); $p; $p = $p->getPrevious()) event('previous:' . $p->getMessage());
}
