<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Finally return suppresses the body exception before local destruction.
require __DIR__ . '/common.php';
class FinallyLocal { public function __destruct() { event('destroy:local'); throw new Exception('destructor'); } }
function finally_return() {
    $local = new FinallyLocal;
    try { throw new Exception('suppressed'); } finally { event('finally'); return 1; }
}
echo 'B';
try { finally_return(); } catch (Exception $e) {
    event('caught:' . $e->getMessage());
    for ($p = $e->getPrevious(); $p; $p = $p->getPrevious()) event('previous:' . $p->getMessage());
}
