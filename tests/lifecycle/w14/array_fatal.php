<?php
// REQ-020/REQ-040; T-039/T-040/T-080. A nested destructor fatal marks siblings immediately.
require __DIR__ . '/common.php';
class ArrayFirst {
    public function __destruct() { event('destroy:array-a'); echo 'A'; trigger_error('array failure', E_USER_ERROR); }
}
function release_array() { $locals = [new ArrayFirst, new Tracked('array-b')]; echo 'B'; }
release_array();
