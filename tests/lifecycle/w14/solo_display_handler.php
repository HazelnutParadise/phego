<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. A diagnostic raised while a user
// output handler runs, and one raised at request shutdown.
ob_start(function (string $buffer): string {
    trigger_error("inside handler", E_USER_WARNING);
    return "[" . $buffer . "]";
});
echo "body";
ob_end_flush();
echo "|";
ob_start(function (string $buffer): string { echo "x"; return $buffer; });
echo "tail";
