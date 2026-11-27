<?php
// fatal_throw_uncaught_code_and_line: exception created on a different line than the throw; its line is printed in finally before the report
function build()
{
    $e = new RuntimeException("built", 77);
    return $e;
}

$x = build();
try {
    throw $x;
} finally {
    echo "line: ", $x->getLine(), " code: ", $x->getCode(), "\n";
}
echo "not reached\n";
