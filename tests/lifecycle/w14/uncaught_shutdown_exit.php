<?php
// REQ-020/REQ-040; T-040/T-080. A later exit changes the exit status after throw.
require __DIR__ . '/common.php';
register_shutdown_function(function () { event("shutdown:exit"); exit(9); });
echo "B";
throw new Exception("body failure");
