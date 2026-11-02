<?php
// REQ-020/REQ-040; T-039/T-080.
require __DIR__ . '/common.php';
echo "B";
throw new Exception("body failure");
