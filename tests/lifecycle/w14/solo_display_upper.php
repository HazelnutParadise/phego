<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. With display_errors=1 PHP writes
// displayed errors through the output layer, so the handler uppercases them.
// ob_start('strtoupper') would fail: PHP passes the handler two arguments.
ob_start(function (string $buffer): string { return strtoupper($buffer); });
echo "body\n";
trigger_error("first warning", E_USER_WARNING);
echo "after\n";
trigger_error("last warning", E_USER_WARNING);
trigger_error("boom", E_USER_ERROR);
