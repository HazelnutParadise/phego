<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Aggregate exceptions preserve every previous exception.
require __DIR__ . '/common.php';
class ChainThrow {
    public function __construct(public string $name) {}
    public function __destruct() { event('destroy:' . $this->name); throw new Exception($this->name); }
}
function chain_release() { $locals = [new ChainThrow('first'), new ChainThrow('second'), new ChainThrow('third')]; }
echo 'B';
try { chain_release(); } catch (Exception $e) {
    event('caught:' . $e->getMessage());
    for ($p = $e->getPrevious(); $p; $p = $p->getPrevious()) event('previous:' . $p->getMessage());
}
