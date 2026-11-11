<?php
// REQ-020/REQ-040; T-039/T-080. E_USER_ERROR is intentionally fatal.
require __DIR__ . '/common.php';
echo "B";
trigger_error("body failure", E_USER_ERROR);
