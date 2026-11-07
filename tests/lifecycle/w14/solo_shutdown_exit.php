<?php
// REQ-040; T-079/T-080. exit in a handler at request shutdown also discards
// every buffer; the exit status is kept.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
ob_start(function ($s) { event("flush:outer"); return "[" . $s . "]"; });
ob_start(function ($s) { event("flush:exit"); echo "x"; exit(3); });
echo "body";
