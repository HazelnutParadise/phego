<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A handled nested exception is not part of the pending chain.
require __DIR__ . '/common.php';
class ChainThrow {
    public function __construct(public string $name) {}
    public function __destruct() { event('destroy:' . $this->name); throw new Exception($this->name); }
}
class CaughtMiddle {
    public function __destruct() { try { throw new Exception('middle'); } catch (Exception $e) { event('caught:middle'); } }
}
function chain_release() { $locals = [new ChainThrow('first'), new CaughtMiddle, new ChainThrow('third')]; }
echo 'B';
try { chain_release(); } catch (Exception $e) {
    event('caught:' . $e->getMessage());
    for ($p = $e->getPrevious(); $p; $p = $p->getPrevious()) event('previous:' . $p->getMessage());
}
