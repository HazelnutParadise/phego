<?php
// REQ-020/REQ-040; T-039/T-040/T-080. Exit differs from a fatal inside array destruction.
require __DIR__ . '/common.php';
class ArrayFirst {
    public function __destruct() { event('destroy:array-a'); echo 'A'; exit(7); }
}
function release_array() { $locals = [new ArrayFirst, new Tracked('array-b')]; echo 'B'; }
release_array();
