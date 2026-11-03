<?php
// REQ-020; T-039/T-040. Both discarded return values can throw from destruction.
require __DIR__ . '/common.php';
class Replacement {
    public function __construct(public string $name) {}
    public function __destruct() { event('destroy:' . $this->name); echo strtoupper($this->name[0]); throw new Exception($this->name); }
}
function replace_return() { try { return new Replacement('old'); } finally { return new Replacement('new'); } }
echo 'B';
try { $result = replace_return(); }
catch (Exception $e) { event('caught:' . $e->getMessage()); event('previous:' . $e->getPrevious()->getMessage()); }
