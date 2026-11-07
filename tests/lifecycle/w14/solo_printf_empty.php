<?php
// REQ-040; T-079/T-080. A zero-length write in a handler still produces
// output for the deprecation, as php_output_handler_op marks any write.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
ob_start(function ($s) { event("flush:inner"); printf(""); return "R"; });
echo "body";
ob_end_flush();
