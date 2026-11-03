<?php
// REQ-040; T-079/T-080. Warning remains non-terminating.
require __DIR__ . '/common.php';
echo "B";
trigger_error("first warning", E_USER_WARNING);
trigger_error("second warning", E_USER_WARNING);
event("body:continued");
