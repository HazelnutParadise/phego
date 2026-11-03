<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Suppression preserves an earlier sibling destructor exception.
require __DIR__ . '/common.php';
class Earlier { public function __destruct() { event('destroy:first'); throw new Exception('first'); } }
class FinallyLocal { public function __destruct() { event('destroy:local'); throw new Exception('destructor'); } }
class FinallyReturn {
    public function __destruct() {
        $local = new FinallyLocal;
        try { throw new Exception('suppressed'); } finally { event('finally'); return; }
    }
}
function release_finally_array() { $local = [new Earlier, new FinallyReturn]; }
echo 'B';
try { release_finally_array(); } catch (Exception $e) {
    event('caught:' . $e->getMessage());
    for ($p = $e->getPrevious(); $p; $p = $p->getPrevious()) event('previous:' . $p->getMessage());
}
