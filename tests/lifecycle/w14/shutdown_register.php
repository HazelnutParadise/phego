<?php
// REQ-020/REQ-040; T-040/T-079. Registration during shutdown is FIFO.
require __DIR__ . '/common.php';
register_shutdown_function(function () {
    event("shutdown:register");
    register_shutdown_function(function () { event("shutdown:late"); });
});
echo "B";
