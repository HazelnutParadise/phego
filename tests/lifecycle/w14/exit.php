<?php
// REQ-020/REQ-040; T-039/T-079.
require __DIR__ . '/common.php';
try { echo "B"; exit(7); } finally { event("finally:unexpected"); }
