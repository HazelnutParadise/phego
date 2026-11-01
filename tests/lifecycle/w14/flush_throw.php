<?php
// REQ-020/REQ-040; T-040/T-080.
require __DIR__ . '/common.php';
ob_start(function ($s) { event("flush:throw"); throw new Exception("flush failure"); });
echo "B";
