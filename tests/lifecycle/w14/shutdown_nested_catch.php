<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Caught nested throws do not stop shutdown destruction.
require __DIR__ . '/common.php';
class NestedChild {
    public function __destruct() { event('destroy:child'); throw new Exception('child failure'); }
}
$objects = [new class {
    public function __destruct() {
        $child = new NestedChild;
        try { unset($child); } catch (Exception $e) { event('caught:child'); }
    }
}, new Tracked('tail')];
echo 'B';
