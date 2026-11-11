<?php
// REQ-040; T-079/T-080. At request shutdown a handler runs without a PHP
// frame; PHP 8.5.12 reports its exception as uncaught and discards the
// buffer instead of printing it with the handler's echo.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
ob_start(function ($s) { event("flush:throw"); echo "x"; throw new Exception("boom"); });
echo "body";
