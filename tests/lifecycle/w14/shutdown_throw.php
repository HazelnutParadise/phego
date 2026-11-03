<?php
// REQ-020/REQ-040; T-040/T-080.
require __DIR__ . '/common.php';
register_shutdown_function(function () { event("shutdown:throw"); throw new Exception("shutdown failure"); });
register_shutdown_function(function () { event("shutdown:unexpected"); });
echo "B";
