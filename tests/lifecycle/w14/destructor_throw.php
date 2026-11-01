<?php
// REQ-020/REQ-040; T-040/T-080.
require __DIR__ . '/common.php';
class Broken {
    public function __destruct() { event("destroy:throw"); throw new Exception("destructor failure"); }
}
$b = new Broken;
echo "B";
