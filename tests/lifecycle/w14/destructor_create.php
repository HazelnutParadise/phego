<?php
// REQ-020/REQ-040; T-040/T-079/T-080. The destructor pass visits new objects.
require __DIR__ . '/common.php';
class Creator {
    public function __destruct() {
        event('destroy:creator');
        $GLOBALS['late'] = new Tracked('late');
        echo 'C';
    }
}
$creator = new Creator;
echo 'B';
