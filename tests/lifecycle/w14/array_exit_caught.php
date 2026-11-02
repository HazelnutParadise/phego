<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Handled cleanup throws must not replace an earlier exit.
require __DIR__ . '/common.php';
class EarlyThrow { public function __destruct() { event('destroy:throw'); throw new Exception('earlier throw'); } }
class CleanupExit { public function __destruct() { event('destroy:exit'); exit(7); } }
class CatchingTail {
    public function __destruct() {
        try { try { throw new Exception('caught tail'); } catch (Exception $e) { event('caught:tail'); echo 'D'; } }
        finally { event('finally'); }
    }
}
function release_caught() { $locals = [new CleanupExit, new CatchingTail]; }
echo 'B';
try { release_caught(); } catch (Exception $e) { event('caught'); echo 'unexpected'; }
